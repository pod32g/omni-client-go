package omni

import (
	"context"
	"testing"
	"time"
)

func TestQueryVector(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"__name__":"up","job":"omni","instance":"127.0.0.1:9090"},"value":[1781538033.843,"1"]},
		{"metric":{"__name__":"up","job":"node","instance":"node-01:9100"},"value":[1781538033.843,"0"]}
	]}}`
	srv := newServer(t, map[string]string{"/api/v1/query": body})
	defer srv.Close()
	c, _ := New(srv.URL)

	res, err := c.Query(context.Background(), "up", time.Unix(1781538033, 0))
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != ResultVector || len(res.Vector) != 2 {
		t.Fatalf("type=%v vector=%d", res.Type, len(res.Vector))
	}
	if res.Vector[0].Value != 1 || res.Vector[0].Metric["job"] != "omni" {
		t.Errorf("sample[0] = %+v", res.Vector[0])
	}
	if res.Vector[1].Value != 0 {
		t.Errorf("sample[1] value = %v, want 0", res.Vector[1].Value)
	}
}

func TestQueryScalar(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"scalar","result":[1781538033.843,"42"]}}`
	srv := newServer(t, map[string]string{"/api/v1/query": body})
	defer srv.Close()
	c, _ := New(srv.URL)

	res, err := c.Query(context.Background(), "42", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != ResultScalar || res.Scalar == nil || res.Scalar.Value != 42 {
		t.Fatalf("scalar result = %+v", res)
	}
}

func TestQueryRangeMatrix(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"matrix","result":[
		{"metric":{"job":"omni"},"values":[[1000,"10"],[2000,"20"],[3000,"30"]]}
	]}}`
	srv := newServer(t, map[string]string{"/api/v1/query_range": body})
	defer srv.Close()
	c, _ := New(srv.URL)

	res, err := c.QueryRange(context.Background(), `m{job="omni"}`, time.Unix(1000, 0), time.Unix(3000, 0), 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != ResultMatrix || len(res.Matrix) != 1 {
		t.Fatalf("matrix = %+v", res)
	}
	pts := res.Matrix[0].Points
	if len(pts) != 3 || pts[0].Value != 10 || pts[2].Value != 30 {
		t.Errorf("points = %+v", pts)
	}
}

func TestQueryRangeRejectsBadStep(t *testing.T) {
	c, _ := New("http://localhost:9090")
	if _, err := c.QueryRange(context.Background(), "up", time.Unix(0, 0), time.Unix(1, 0), 0); err == nil {
		t.Error("zero step should error")
	}
}

func TestQueryPropagatesAPIError(t *testing.T) {
	body := `{"status":"error","errorType":"bad_data","error":"boom"}`
	srv := newServer(t, map[string]string{"/api/v1/query": body})
	defer srv.Close()
	c, _ := New(srv.URL)
	if _, err := c.Query(context.Background(), "bad{", time.Time{}); err == nil {
		t.Error("API error should propagate from Query")
	}
}
