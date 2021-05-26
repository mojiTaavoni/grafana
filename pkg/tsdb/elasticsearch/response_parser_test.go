package elasticsearch

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/grafana/grafana/pkg/components/null"
	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/plugins"
	es "github.com/grafana/grafana/pkg/tsdb/elasticsearch/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResponseParser(t *testing.T) {
	t.Run("Elasticsearch response parser test", func(t *testing.T) {
		t.Run("Simple query and count", func(t *testing.T) {
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [{ "type": "count", "id": "1" }],
          "bucketAggs": [{ "type": "date_histogram", "field": "@timestamp", "id": "2" }]
				}`,
			}
			response := `{
        "responses": [
          {
            "aggregations": {
              "2": {
                "buckets": [
                  {
                    "doc_count": 10,
                    "key": 1000
                  },
                  {
                    "doc_count": 15,
                    "key": 2000
                  }
                ]
              }
            }
          }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			require.NoError(t, err)
			result, err := rp.getTimeSeries()
			require.NoError(t, err)
			require.Len(t, result.Results, 1)

			queryRes := result.Results["A"]
			require.NotNil(t, queryRes)
			dataframes, err := queryRes.Dataframes.Decoded()
			require.NoError(t, err)
			require.Len(t, dataframes, 1)

			frame := dataframes[0]
			require.Equal(t, frame.Name, "Count")
			require.Len(t, frame.Fields, 2)

			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)
		})

		t.Run("Simple query count & avg aggregation", func(t *testing.T) {
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [{ "type": "count", "id": "1" }, {"type": "avg", "field": "value", "id": "2" }],
          "bucketAggs": [{ "type": "date_histogram", "field": "@timestamp", "id": "3" }]
				}`,
			}
			response := `{
        "responses": [
          {
            "aggregations": {
              "3": {
                "buckets": [
                  {
                    "2": { "value": 88 },
                    "doc_count": 10,
                    "key": 1000
                  },
                  {
                    "2": { "value": 99 },
                    "doc_count": 15,
                    "key": 2000
                  }
                ]
              }
            }
          }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			require.NoError(t, err)
			result, err := rp.getTimeSeries()
			require.NoError(t, err)
			require.Len(t, result.Results, 1)

			queryRes := result.Results["A"]
			require.NotNil(t, queryRes)
			dataframes, err := queryRes.Dataframes.Decoded()
			require.NoError(t, err)
			require.Len(t, dataframes, 2)

			frame := dataframes[0]
			require.Equal(t, frame.Name, "Count")
			require.Len(t, frame.Fields, 2)

			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[1]
			require.Equal(t, frame.Name, "Average value")
			require.Len(t, frame.Fields, 2)

			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)
		})

		t.Run("Single group by query one metric", func(t *testing.T) {
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [{ "type": "count", "id": "1" }],
          "bucketAggs": [
						{ "type": "terms", "field": "host", "id": "2" },
						{ "type": "date_histogram", "field": "@timestamp", "id": "3" }
					]
				}`,
			}
			response := `{
        "responses": [
          {
            "aggregations": {
              "2": {
                "buckets": [
                  {
                    "3": {
                      "buckets": [{ "doc_count": 1, "key": 1000 }, { "doc_count": 3, "key": 2000 }]
                    },
                    "doc_count": 4,
                    "key": "server1"
                  },
                  {
                    "3": {
                      "buckets": [{ "doc_count": 2, "key": 1000 }, { "doc_count": 8, "key": 2000 }]
                    },
                    "doc_count": 10,
                    "key": "server2"
                  }
                ]
              }
            }
          }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			require.NoError(t, err)
			result, err := rp.getTimeSeries()
			require.NoError(t, err)

			queryRes := result.Results["A"]
			require.NotNil(t, queryRes)
			dataframes, err := queryRes.Dataframes.Decoded()
			require.NoError(t, err)
			require.Len(t, dataframes, 2)

			frame := dataframes[0]
			require.Equal(t, frame.Name, "server1")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[1]
			require.Equal(t, frame.Name, "server2")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)
		})

		t.Run("Single group by query two metrics", func(t *testing.T) {
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [{ "type": "count", "id": "1" }, { "type": "avg", "field": "@value", "id": "4" }],
          "bucketAggs": [
						{ "type": "terms", "field": "host", "id": "2" },
						{ "type": "date_histogram", "field": "@timestamp", "id": "3" }
					]
				}`,
			}
			response := `{
        "responses": [
          {
            "aggregations": {
              "2": {
                "buckets": [
                  {
                    "3": {
                      "buckets": [
                        { "4": { "value": 10 }, "doc_count": 1, "key": 1000 },
                        { "4": { "value": 12 }, "doc_count": 3, "key": 2000 }
                      ]
                    },
                    "doc_count": 4,
                    "key": "server1"
                  },
                  {
                    "3": {
                      "buckets": [
                        { "4": { "value": 20 }, "doc_count": 1, "key": 1000 },
                        { "4": { "value": 32 }, "doc_count": 3, "key": 2000 }
                      ]
                    },
                    "doc_count": 10,
                    "key": "server2"
                  }
                ]
              }
            }
          }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			require.NoError(t, err)
			result, err := rp.getTimeSeries()
			require.NoError(t, err)
			require.Len(t, result.Results, 1)

			queryRes := result.Results["A"]
			require.NotNil(t, queryRes)
			dataframes, err := queryRes.Dataframes.Decoded()
			require.NoError(t, err)
			require.Len(t, dataframes, 4)

			frame := dataframes[0]
			require.Equal(t, frame.Name, "server1 Count")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[1]
			require.Equal(t, frame.Name, "server1 Average @value")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[2]
			require.Equal(t, frame.Name, "server2 Count")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[3]
			require.Equal(t, frame.Name, "server2 Average @value")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)
		})

		t.Run("With percentiles", func(t *testing.T) {
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [{ "type": "percentiles", "settings": { "percents": [75, 90] }, "id": "1" }],
          "bucketAggs": [{ "type": "date_histogram", "field": "@timestamp", "id": "3" }]
				}`,
			}
			response := `{
        "responses": [
          {
            "aggregations": {
              "3": {
                "buckets": [
                  {
                    "1": { "values": { "75": 3.3, "90": 5.5 } },
                    "doc_count": 10,
                    "key": 1000
                  },
                  {
                    "1": { "values": { "75": 2.3, "90": 4.5 } },
                    "doc_count": 15,
                    "key": 2000
                  }
                ]
              }
            }
          }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			require.NoError(t, err)
			result, err := rp.getTimeSeries()
			require.NoError(t, err)
			require.Len(t, result.Results, 1)

			queryRes := result.Results["A"]
			require.NotNil(t, queryRes)
			dataframes, err := queryRes.Dataframes.Decoded()
			require.NoError(t, err)
			require.Len(t, dataframes, 2)

			frame := dataframes[0]
			require.Equal(t, frame.Name, "p75")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[1]
			require.Equal(t, frame.Name, "p90")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 4)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 4)
		})

		t.Run("With extended stats", func(t *testing.T) {
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [{ "type": "extended_stats", "meta": { "max": true, "std_deviation_bounds_upper": true, "std_deviation_bounds_lower": true }, "id": "1" }],
          "bucketAggs": [
						{ "type": "terms", "field": "host", "id": "3" },
						{ "type": "date_histogram", "field": "@timestamp", "id": "4" }
					]
				}`,
			}
			response := `{
        "responses": [
          {
            "aggregations": {
              "3": {
                "buckets": [
                  {
                    "key": "server1",
                    "4": {
                      "buckets": [
                        {
                          "1": {
                            "max": 10.2,
                            "min": 5.5,
                            "std_deviation_bounds": { "upper": 3, "lower": -2 }
                          },
                          "doc_count": 10,
                          "key": 1000
                        }
                      ]
                    }
                  },
                  {
                    "key": "server2",
                    "4": {
                      "buckets": [
                        {
                          "1": {
                            "max": 15.5,
                            "min": 3.4,
                            "std_deviation_bounds": { "upper": 4, "lower": -1 }
                          },
                          "doc_count": 10,
                          "key": 1000
                        }
                      ]
                    }
                  }
                ]
              }
            }
          }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			require.NoError(t, err)
			result, err := rp.getTimeSeries()
			require.NoError(t, err)
			require.Len(t, result.Results, 1)

			queryRes := result.Results["A"]
			require.NotNil(t, queryRes)
			dataframes, err := queryRes.Dataframes.Decoded()
			require.NoError(t, err)
			require.Len(t, dataframes, 6)

			frame := dataframes[0]
			require.Equal(t, frame.Name, "server1 Max")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 1)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 1)

			frame = dataframes[1]
			require.Equal(t, frame.Name, "server1 Std Dev Lower")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[2]
			require.Equal(t, frame.Name, "server1 Std Dev Upper")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 3)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 3)

			frame = dataframes[3]
			require.Equal(t, frame.Name, "server2 Max")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 1)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 1)

			frame = dataframes[4]
			require.Equal(t, frame.Name, "server2 Std Dev Lower")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[5]
			require.Equal(t, frame.Name, "server2 Std Dev Upper")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 3)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 3)
		})

		t.Run("Single group by with alias pattern", func(t *testing.T) {
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"alias": "{{term @host}} {{metric}} and {{not_exist}} {{@host}}",
					"metrics": [{ "type": "count", "id": "1" }],
          "bucketAggs": [
						{ "type": "terms", "field": "@host", "id": "2" },
						{ "type": "date_histogram", "field": "@timestamp", "id": "3" }
					]
				}`,
			}
			response := `{
        "responses": [
          {
            "aggregations": {
              "2": {
                "buckets": [
                  {
                    "3": {
                      "buckets": [{ "doc_count": 1, "key": 1000 }, { "doc_count": 3, "key": 2000 }]
                    },
                    "doc_count": 4,
                    "key": "server1"
                  },
                  {
                    "3": {
                      "buckets": [{ "doc_count": 2, "key": 1000 }, { "doc_count": 8, "key": 2000 }]
                    },
                    "doc_count": 10,
                    "key": "server2"
                  },
                  {
                    "3": {
                      "buckets": [{ "doc_count": 2, "key": 1000 }, { "doc_count": 8, "key": 2000 }]
                    },
                    "doc_count": 10,
                    "key": 0
                  }
                ]
              }
            }
          }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			require.NoError(t, err)
			result, err := rp.getTimeSeries()
			require.NoError(t, err)
			require.Len(t, result.Results, 1)

			queryRes := result.Results["A"]
			require.NotNil(t, queryRes)
			dataframes, err := queryRes.Dataframes.Decoded()
			require.NoError(t, err)
			require.Len(t, dataframes, 3)

			frame := dataframes[0]
			require.Equal(t, frame.Name, "server1 Count and {{not_exist}} server1")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[1]
			require.Equal(t, frame.Name, "server2 Count and {{not_exist}} server2")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[2]
			require.Equal(t, frame.Name, "0 Count and {{not_exist}} 0")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)
		})

		t.Run("Histogram response", func(t *testing.T) {
			t.Skip()
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [{ "type": "count", "id": "1" }],
         "bucketAggs": [{ "type": "histogram", "field": "bytes", "id": "3" }]
				}`,
			}
			response := `{
        "responses": [
         {
           "aggregations": {
             "3": {
               "buckets": [{ "doc_count": 1, "key": 1000 }, { "doc_count": 3, "key": 2000 }, { "doc_count": 2, "key": 3000 }]
             }
           }
         }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			require.NoError(t, err)
			result, err := rp.getTimeSeries()
			require.NoError(t, err)
			require.Len(t, result.Results, 1)

			queryRes := result.Results["A"]
			require.NotNil(t, queryRes)
			require.Len(t, queryRes.Tables, 1)

			rows := queryRes.Tables[0].Rows
			require.Len(t, rows, 3)
			cols := queryRes.Tables[0].Columns
			require.Len(t, cols, 2)

			require.Equal(t, cols[0].Text, "bytes")
			require.Equal(t, cols[1].Text, "Count")

			require.Equal(t, rows[0][0].(null.Float).Float64, 1000)
			require.Equal(t, rows[0][1].(null.Float).Float64, 1)
			require.Equal(t, rows[1][0].(null.Float).Float64, 2000)
			require.Equal(t, rows[1][1].(null.Float).Float64, 3)
			require.Equal(t, rows[2][0].(null.Float).Float64, 3000)
			require.Equal(t, rows[2][1].(null.Float).Float64, 2)
		})

		t.Run("With two filters agg", func(t *testing.T) {
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [{ "type": "count", "id": "1" }],
          "bucketAggs": [
						{
							"type": "filters",
							"id": "2",
							"settings": {
								"filters": [{ "query": "@metric:cpu" }, { "query": "@metric:logins.count" }]
							}
						},
						{ "type": "date_histogram", "field": "@timestamp", "id": "3" }
					]
				}`,
			}
			response := `{
        "responses": [
          {
            "aggregations": {
              "2": {
                "buckets": {
                  "@metric:cpu": {
                    "3": {
                      "buckets": [{ "doc_count": 1, "key": 1000 }, { "doc_count": 3, "key": 2000 }]
                    }
                  },
                  "@metric:logins.count": {
                    "3": {
                      "buckets": [{ "doc_count": 2, "key": 1000 }, { "doc_count": 8, "key": 2000 }]
                    }
                  }
                }
              }
            }
          }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			require.NoError(t, err)
			result, err := rp.getTimeSeries()
			require.NoError(t, err)
			require.Len(t, result.Results, 1)

			queryRes := result.Results["A"]
			require.NotNil(t, queryRes)
			dataframes, err := queryRes.Dataframes.Decoded()
			require.NoError(t, err)
			require.Len(t, dataframes, 2)

			frame := dataframes[0]
			require.Equal(t, frame.Name, "@metric:cpu")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[1]
			require.Equal(t, frame.Name, "@metric:logins.count")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)
		})

		t.Run("With dropfirst and last aggregation", func(t *testing.T) {
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [{ "type": "avg", "id": "1" }, { "type": "count" }],
          "bucketAggs": [
						{
							"type": "date_histogram",
							"field": "@timestamp",
							"id": "2",
							"settings": { "trimEdges": 1 }
						}
					]
				}`,
			}
			response := `{
        "responses": [
          {
            "aggregations": {
              "2": {
                "buckets": [
                  {
                    "1": { "value": 1000 },
                    "key": 1,
                    "doc_count": 369
                  },
                  {
                    "1": { "value": 2000 },
                    "key": 2,
                    "doc_count": 200
                  },
                  {
                    "1": { "value": 2000 },
                    "key": 3,
                    "doc_count": 200
                  }
                ]
              }
            }
          }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			require.NoError(t, err)
			result, err := rp.getTimeSeries()
			require.NoError(t, err)
			require.Len(t, result.Results, 1)

			queryRes := result.Results["A"]
			require.NotNil(t, queryRes)
			dataframes, err := queryRes.Dataframes.Decoded()
			require.NoError(t, err)
			require.Len(t, dataframes, 2)

			frame := dataframes[0]
			require.Equal(t, frame.Name, "Average")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[1]
			require.Equal(t, frame.Name, "Count")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)
		})

		t.Run("No group by time", func(t *testing.T) {
			t.Skip()
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [{ "type": "avg", "id": "1" }, { "type": "count" }],
         "bucketAggs": [{ "type": "terms", "field": "host", "id": "2" }]
				}`,
			}
			response := `{
        "responses": [
         {
           "aggregations": {
             "2": {
               "buckets": [
                 {
                   "1": { "value": 1000 },
                   "key": "server-1",
                   "doc_count": 369
                 },
                 {
                   "1": { "value": 2000 },
                   "key": "server-2",
                   "doc_count": 200
                 }
               ]
             }
           }
         }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			So(err, ShouldBeNil)
			result, err := rp.getTimeSeries()
			So(err, ShouldBeNil)
			So(result.Results, ShouldHaveLength, 1)

			queryRes := result.Results["A"]
			So(queryRes, ShouldNotBeNil)
			So(queryRes.Tables, ShouldHaveLength, 1)

			rows := queryRes.Tables[0].Rows
			So(rows, ShouldHaveLength, 2)
			cols := queryRes.Tables[0].Columns
			So(cols, ShouldHaveLength, 3)

			So(cols[0].Text, ShouldEqual, "host")
			So(cols[1].Text, ShouldEqual, "Average")
			So(cols[2].Text, ShouldEqual, "Count")

			So(rows[0][0].(string), ShouldEqual, "server-1")
			So(rows[0][1].(null.Float).Float64, ShouldEqual, 1000)
			So(rows[0][2].(null.Float).Float64, ShouldEqual, 369)
			So(rows[1][0].(string), ShouldEqual, "server-2")
			So(rows[1][1].(null.Float).Float64, ShouldEqual, 2000)
			So(rows[1][2].(null.Float).Float64, ShouldEqual, 200)
		})

		t.Run("Multiple metrics of same type", func(t *testing.T) {
			t.Skip()
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [{ "type": "avg", "field": "test", "id": "1" }, { "type": "avg", "field": "test2", "id": "2" }],
          "bucketAggs": [{ "type": "terms", "field": "host", "id": "2" }]
				}`,
			}
			response := `{
        "responses": [
          {
            "aggregations": {
              "2": {
                "buckets": [
                  {
                    "1": { "value": 1000 },
                    "2": { "value": 3000 },
                    "key": "server-1",
                    "doc_count": 369
                  }
                ]
              }
            }
          }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			So(err, ShouldBeNil)
			result, err := rp.getTimeSeries()
			So(err, ShouldBeNil)
			So(result.Results, ShouldHaveLength, 1)

			queryRes := result.Results["A"]
			So(queryRes, ShouldNotBeNil)
			So(queryRes.Tables, ShouldHaveLength, 1)

			rows := queryRes.Tables[0].Rows
			So(rows, ShouldHaveLength, 1)
			cols := queryRes.Tables[0].Columns
			So(cols, ShouldHaveLength, 3)

			So(cols[0].Text, ShouldEqual, "host")
			So(cols[1].Text, ShouldEqual, "Average test")
			So(cols[2].Text, ShouldEqual, "Average test2")

			So(rows[0][0].(string), ShouldEqual, "server-1")
			So(rows[0][1].(null.Float).Float64, ShouldEqual, 1000)
			So(rows[0][2].(null.Float).Float64, ShouldEqual, 3000)
		})

		t.Run("With bucket_script", func(t *testing.T) {
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [
						{ "id": "1", "type": "sum", "field": "@value" },
            { "id": "3", "type": "max", "field": "@value" },
            {
              "id": "4",
              "field": "select field",
              "pipelineVariables": [{ "name": "var1", "pipelineAgg": "1" }, { "name": "var2", "pipelineAgg": "3" }],
              "settings": { "script": "params.var1 * params.var2" },
              "type": "bucket_script"
            }
					],
          "bucketAggs": [{ "type": "date_histogram", "field": "@timestamp", "id": "2" }]
				}`,
			}
			response := `{
        "responses": [
          {
            "aggregations": {
              "2": {
                "buckets": [
                  {
                    "1": { "value": 2 },
                    "3": { "value": 3 },
                    "4": { "value": 6 },
                    "doc_count": 60,
                    "key": 1000
                  },
                  {
                    "1": { "value": 3 },
                    "3": { "value": 4 },
                    "4": { "value": 12 },
                    "doc_count": 60,
                    "key": 2000
                  }
                ]
              }
            }
          }
        ]
			}`
			rp, err := newResponseParserForTest(targets, response)
			require.NoError(t, err)
			result, err := rp.getTimeSeries()
			require.NoError(t, err)
			require.Len(t, result.Results, 1)

			queryRes := result.Results["A"]
			require.NotNil(t, queryRes)
			dataframes, err := queryRes.Dataframes.Decoded()
			require.NoError(t, err)
			require.Len(t, dataframes, 3)

			frame := dataframes[0]
			require.Equal(t, frame.Name, "Sum @value")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[1]
			require.Equal(t, frame.Name, "Max @value")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)

			frame = dataframes[2]
			require.Equal(t, frame.Name, "Sum @value * Max @value")
			require.Len(t, frame.Fields, 2)
			require.Equal(t, frame.Fields[0].Name, "time")
			require.Equal(t, frame.Fields[0].Len(), 2)
			require.Equal(t, frame.Fields[1].Name, "value")
			require.Equal(t, frame.Fields[1].Len(), 2)
		})

		t.Run("Terms with two bucket_script", func(t *testing.T) {
			t.Skip()
			targets := map[string]string{
				"A": `{
					"timeField": "@timestamp",
					"metrics": [
						{ "id": "1", "type": "sum", "field": "@value" },
            			{ "id": "3", "type": "max", "field": "@value" },
            			{
              				"id": "4",
              				"field": "select field",
              				"pipelineVariables": [{ "name": "var1", "pipelineAgg": "1" }, { "name": "var2", "pipelineAgg": "3" }],
              				"settings": { "script": "params.var1 * params.var2" },
              				"type": "bucket_script"
						},
            			{
							"id": "5",
							"field": "select field",
							"pipelineVariables": [{ "name": "var1", "pipelineAgg": "1" }, { "name": "var2", "pipelineAgg": "3" }],
							"settings": { "script": "params.var1 * params.var2 * 2" },
							"type": "bucket_script"
					  }
					],
          "bucketAggs": [{ "type": "terms", "field": "@timestamp", "id": "2" }]
				}`,
			}
			response := `{
				"responses": [
					{
						"aggregations": {
						"2": {
							"buckets": [
							{
								"1": { "value": 2 },
								"3": { "value": 3 },
								"4": { "value": 6 },
								"5": { "value": 24 },
								"doc_count": 60,
								"key": 1000
							},
							{
								"1": { "value": 3 },
								"3": { "value": 4 },
								"4": { "value": 12 },
								"5": { "value": 48 },
								"doc_count": 60,
								"key": 2000
							}
							]
						}
						}
					}
				]
			}`
			rp, err := newResponseParserForTest(targets, response)
			So(err, ShouldBeNil)
			result, err := rp.getTimeSeries()
			So(err, ShouldBeNil)
			So(result.Results, ShouldHaveLength, 1)
			queryRes := result.Results["A"]
			So(queryRes, ShouldNotBeNil)
			So(queryRes.Tables[0].Rows, ShouldHaveLength, 2)
			So(queryRes.Tables[0].Columns[1].Text, ShouldEqual, "Sum")
			So(queryRes.Tables[0].Columns[2].Text, ShouldEqual, "Max")
			So(queryRes.Tables[0].Columns[3].Text, ShouldEqual, "params.var1 * params.var2")
			So(queryRes.Tables[0].Columns[4].Text, ShouldEqual, "params.var1 * params.var2 * 2")
			So(queryRes.Tables[0].Rows[0][1].(null.Float).Float64, ShouldEqual, 2)
			So(queryRes.Tables[0].Rows[0][2].(null.Float).Float64, ShouldEqual, 3)
			So(queryRes.Tables[0].Rows[0][3].(null.Float).Float64, ShouldEqual, 6)
			So(queryRes.Tables[0].Rows[0][4].(null.Float).Float64, ShouldEqual, 24)
			So(queryRes.Tables[0].Rows[1][1].(null.Float).Float64, ShouldEqual, 3)
			So(queryRes.Tables[0].Rows[1][2].(null.Float).Float64, ShouldEqual, 4)
			So(queryRes.Tables[0].Rows[1][3].(null.Float).Float64, ShouldEqual, 12)
			So(queryRes.Tables[0].Rows[1][4].(null.Float).Float64, ShouldEqual, 48)
		})
		// t.Run("Raw documents query", func(t *testing.T) {
		// 	targets := map[string]string{
		// 		"A": `{
		// 			"timeField": "@timestamp",
		// 			"metrics": [{ "type": "raw_document", "id": "1" }]
		// 		}`,
		// 	}
		// 	response := `{
		//     "responses": [
		//       {
		//         "hits": {
		//           "total": 100,
		//           "hits": [
		//             {
		//               "_id": "1",
		//               "_type": "type",
		//               "_index": "index",
		//               "_source": { "sourceProp": "asd" },
		//               "fields": { "fieldProp": "field" }
		//             },
		//             {
		//               "_source": { "sourceProp": "asd2" },
		//               "fields": { "fieldProp": "field2" }
		//             }
		//           ]
		//         }
		//       }
		//     ]
		// 	}`
		// 	rp, err := newResponseParserForTest(targets, response)
		// 	So(err, ShouldBeNil)
		// 	result, err := rp.getTimeSeries()
		// 	So(err, ShouldBeNil)
		// 	So(result.Results, ShouldHaveLength, 1)

		// 	queryRes := result.Results["A"]
		// 	So(queryRes, ShouldNotBeNil)
		// 	So(queryRes.Tables, ShouldHaveLength, 1)

		// 	rows := queryRes.Tables[0].Rows
		// 	So(rows, ShouldHaveLength, 1)
		// 	cols := queryRes.Tables[0].Columns
		// 	So(cols, ShouldHaveLength, 3)

		// 	So(cols[0].Text, ShouldEqual, "host")
		// 	So(cols[1].Text, ShouldEqual, "Average test")
		// 	So(cols[2].Text, ShouldEqual, "Average test2")

		// 	So(rows[0][0].(string), ShouldEqual, "server-1")
		// 	So(rows[0][1].(null.Float).Float64, ShouldEqual, 1000)
		// 	So(rows[0][2].(null.Float).Float64, ShouldEqual, 3000)
		// })
	})

	t.Run("With top_metrics", func(t *testing.T) {
		targets := map[string]string{
			"A": `{
				"timeField": "@timestamp",
				"metrics": [
					{
						"type": "top_metrics",
						"settings": {
							"order": "desc",
							"orderBy": "@timestamp",
							"metrics": ["@value", "@anotherValue"]
						},
						"id": "1"
					}
				],
				"bucketAggs": [{ "type": "date_histogram", "field": "@timestamp", "id": "3" }]
			}`,
		}
		response := `{
			"responses": [{
				"aggregations": {
					"3": {
						"buckets": [
							{
								"key": 1609459200000,
								"key_as_string": "2021-01-01T00:00:00.000Z",
								"1": {
									"top": [
										{ "sort": ["2021-01-01T00:00:00.000Z"], "metrics": { "@value": 1, "@anotherValue": 2 } }
									]
								}
							},
							{
								"key": 1609459210000,
								"key_as_string": "2021-01-01T00:00:10.000Z",
								"1": {
									"top": [
										{ "sort": ["2021-01-01T00:00:10.000Z"], "metrics": { "@value": 1, "@anotherValue": 2 } }
									]
								}
							}
						]			
					}
				}
			}]
		}`
		rp, err := newResponseParserForTest(targets, response)
		assert.Nil(t, err)
		result, err := rp.getTimeSeries()
		assert.Nil(t, err)
		assert.Len(t, result.Results, 1)

		queryRes := result.Results["A"]
		assert.NotNil(t, queryRes)
		assert.Len(t, queryRes.Series, 2)

		seriesOne := queryRes.Series[0]
		assert.Equal(t, seriesOne.Name, "Top Metrics @value")
		assert.Len(t, seriesOne.Points, 2)
		assert.Equal(t, seriesOne.Points[0][0].Float64, 1.)
		assert.Equal(t, seriesOne.Points[0][1].Float64, 1609459200000.)
		assert.Equal(t, seriesOne.Points[1][0].Float64, 1.)
		assert.Equal(t, seriesOne.Points[1][1].Float64, 1609459210000.)

		seriesTwo := queryRes.Series[1]
		assert.Equal(t, seriesTwo.Name, "Top Metrics @anotherValue")
		assert.Len(t, seriesTwo.Points, 2)

		assert.Equal(t, seriesTwo.Points[0][0].Float64, 2.)
		assert.Equal(t, seriesTwo.Points[0][1].Float64, 1609459200000.)
		assert.Equal(t, seriesTwo.Points[1][0].Float64, 2.)
		assert.Equal(t, seriesTwo.Points[1][1].Float64, 1609459210000.)
	})
}

func newResponseParserForTest(tsdbQueries map[string]string, responseBody string) (*responseParser, error) {
	from := time.Date(2018, 5, 15, 17, 50, 0, 0, time.UTC)
	to := time.Date(2018, 5, 15, 17, 55, 0, 0, time.UTC)
	fromStr := fmt.Sprintf("%d", from.UnixNano()/int64(time.Millisecond))
	toStr := fmt.Sprintf("%d", to.UnixNano()/int64(time.Millisecond))
	timeRange := plugins.NewDataTimeRange(fromStr, toStr)
	tsdbQuery := plugins.DataQuery{
		Queries:   []plugins.DataSubQuery{},
		TimeRange: &timeRange,
	}

	for refID, tsdbQueryBody := range tsdbQueries {
		tsdbQueryJSON, err := simplejson.NewJson([]byte(tsdbQueryBody))
		if err != nil {
			return nil, err
		}

		tsdbQuery.Queries = append(tsdbQuery.Queries, plugins.DataSubQuery{
			Model: tsdbQueryJSON,
			RefID: refID,
		})
	}

	var response es.MultiSearchResponse
	err := json.Unmarshal([]byte(responseBody), &response)
	if err != nil {
		return nil, err
	}

	tsQueryParser := newTimeSeriesQueryParser()
	queries, err := tsQueryParser.parse(tsdbQuery)
	if err != nil {
		return nil, err
	}

	return newResponseParser(response.Responses, queries, nil), nil
}
