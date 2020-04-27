package adapter

import (
	"context"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/handlers"
	"github.com/julienschmidt/httprouter"
)

type timeSeries struct {
	Labels  []*label  `bson:"labels,omitempty"`
	Samples []*sample `bson:"samples,omitempty"`
}

type label struct {
	Name  string `bson:"name,omitempty"`
	Value string `bson:"value,omitempty"`
}

type sample struct {
	Timestamp int64   `bson:"timestamp"`
	Value     float64 `bson:"value"`
}

// MongoDBAdapter is an implemantation of prometheus remote stprage adapter for MongoDB
type MongoDBAdapter struct {
	client *mongo.Client
	coll   *mongo.Collection
}

// New provides a MongoDBAdapter after initialization
func New(urlString, database string, collection string) (*MongoDBAdapter, error) {

	u, err := url.Parse(urlString)
	if err != nil {
		return nil, fmt.Errorf("url parse error: %s", err.Error())
	}
	u.RawQuery = ""

	// 初始化连接参数
	client, err := mongo.NewClient(options.Client().ApplyURI(u.String()))
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err = client.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("mongo url parse error: %s", err.Error())
	}
	err = client.Ping(ctx, readpref.Primary())
	if err != nil {
		return nil, fmt.Errorf("mongo server is offline: %s", err.Error())
	}

	// 获取数据库和表名
	// 返回adapter
	c := client.Database(database).Collection(collection)

	return &MongoDBAdapter{
		client: client,
		coll:   c,
	}, nil
}

// Close closes the connection with MongoDB
func (adapter *MongoDBAdapter) Close() {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err := adapter.client.Disconnect(ctx)
	if err != nil {
		logrus.Error("ERROR in close client", err)
	}
}

// Run serves with http listener
func (adapter *MongoDBAdapter) Run(address string) error {
	router := httprouter.New()
	router.POST("/write", adapter.handleWriteRequest)
	router.POST("/read", adapter.handleReadRequest)
	return http.ListenAndServe(address, handlers.RecoveryHandler()(handlers.LoggingHandler(os.Stdout, router)))
}

func (adapter *MongoDBAdapter) handleWriteRequest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logrus.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		logrus.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		logrus.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, ts := range req.Timeseries {
		mongoTS := &timeSeries{
			Labels:  []*label{},
			Samples: []*sample{},
		}
		for _, l := range ts.Labels {
			mongoTS.Labels = append(mongoTS.Labels, &label{
				Name:  l.Name,
				Value: l.Value,
			})
		}
		for _, s := range ts.Samples {
			mongoTS.Samples = append(mongoTS.Samples, &sample{
				Timestamp: s.Timestamp,
				Value:     s.Value,
			})
		}

		logrus.Debug("Try to insert: ", mongoTS)
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		if _, err := adapter.coll.InsertOne(ctx, mongoTS); err != nil {
			logrus.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
}

func (adapter *MongoDBAdapter) handleReadRequest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	compressed, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logrus.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reqBuf, err := snappy.Decode(nil, compressed)
	if err != nil {
		logrus.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req prompb.ReadRequest
	if err := proto.Unmarshal(reqBuf, &req); err != nil {
		logrus.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	results := []*prompb.QueryResult{}
	for _, q := range req.Queries {

		query := bson.M{
			"samples": bson.M{
				"$elemMatch": bson.M{
					"timestamp": bson.M{
						"$gte": q.StartTimestampMs,
						"$lte": q.EndTimestampMs,
					},
				},
			},
		}
		if q.Matchers != nil && len(q.Matchers) > 0 {
			matcher := []bson.M{}
			for _, m := range q.Matchers {
				switch m.Type {
				case prompb.LabelMatcher_EQ:
					matcher = append(matcher, bson.M{
						"$elemMatch": bson.M{
							m.Name: m.Value,
						},
					})
				case prompb.LabelMatcher_NEQ:
					matcher = append(matcher, bson.M{
						"$elemMatch": bson.M{
							m.Name: bson.M{
								"$ne": m.Value,
							},
						},
					})
				case prompb.LabelMatcher_RE:
					matcher = append(matcher, bson.M{
						"$elemMatch": bson.M{
							m.Name: bson.M{
								"$regex": m.Value,
							},
						},
					})
				case prompb.LabelMatcher_NRE:
					matcher = append(matcher, bson.M{
						"$elemMatch": bson.M{
							m.Name: bson.M{
								"$not": bson.M{
									"$regex": m.Value,
								},
							},
						},
					})
				}
			}
			query["labels"] = bson.M{
				"$all": matcher,
			}
		}

		findOptions := options.Find()
		findOptions.SetSort(bson.M{"samples.timestamp": -1})
		ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
		cursor, err := adapter.coll.Find(ctx, query, findOptions)
		if err != nil {
			logrus.Error("ERROR in find by query", query, err)
		}
		defer cursor.Close(ctx)

		timeseries := []*prompb.TimeSeries{}
		for cursor.Next(ctx) {
			timeseries = append(timeseries, &prompb.TimeSeries{})
		}

		results = append(results, &prompb.QueryResult{
			Timeseries: timeseries,
		})
	}
	resp := &prompb.ReadResponse{
		Results: results,
	}
	data, err := proto.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Header().Set("Content-Encoding", "snappy")
	compressed = snappy.Encode(nil, data)
	if _, err := w.Write(compressed); err != nil {
		logrus.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
