package omni

import (
	"context"
	"testing"
	"time"
)

func TestSeries(t *testing.T) {
	body := `{"status":"success","data":[
		{"__name__":"up","job":"omni","instance":"127.0.0.1:9090"},
		{"__name__":"up","job":"node","instance":"node-01:9100"}
	]}`
	srv := newServer(t, map[string]string{"/api/v1/series": body})
	defer srv.Close()
	c, _ := New(srv.URL)

	got, err := c.Series(context.Background(), []string{"up"}, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["job"] != "omni" {
		t.Errorf("series = %+v", got)
	}
}

func TestSeriesRequiresMatch(t *testing.T) {
	c, _ := New("http://localhost:9090")
	if _, err := c.Series(context.Background(), nil, time.Time{}, time.Time{}); err == nil {
		t.Error("Series with no match selectors should error")
	}
}

func TestLabelNamesAndValues(t *testing.T) {
	srv := newServer(t, map[string]string{
		"/api/v1/labels":           `{"status":"success","data":["__name__","instance","job"]}`,
		"/api/v1/label/job/values": `{"status":"success","data":["node","omni"]}`,
	})
	defer srv.Close()
	c, _ := New(srv.URL)

	names, err := c.LabelNames(context.Background())
	if err != nil || len(names) != 3 {
		t.Fatalf("LabelNames = %v, %v", names, err)
	}
	vals, err := c.LabelValues(context.Background(), "job")
	if err != nil || len(vals) != 2 || vals[1] != "omni" {
		t.Fatalf("LabelValues = %v, %v", vals, err)
	}
	if _, err := c.LabelValues(context.Background(), ""); err == nil {
		t.Error("empty label name should error")
	}
}

func TestTargets(t *testing.T) {
	body := `{"status":"success","data":[
		{"job":"omni","instance":"127.0.0.1:9090","scrapeUrl":"http://127.0.0.1:9090/metrics","up":true,"lastScrape":"2026-06-15T15:40:28.052269477Z","lastError":"","lastScrapeDuration":0.00086845,"samplesScraped":6},
		{"job":"redis","instance":"cache-02:9121","scrapeUrl":"http://cache-02:9121/metrics","up":false,"lastScrape":"2026-06-15T15:40:14Z","lastError":"connection refused","lastScrapeDuration":0,"samplesScraped":0}
	]}`
	srv := newServer(t, map[string]string{"/api/v1/targets": body})
	defer srv.Close()
	c, _ := New(srv.URL)

	targets, err := c.Targets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %d", len(targets))
	}
	if !targets[0].Up || targets[0].Job != "omni" || targets[0].SamplesScraped != 6 {
		t.Errorf("target[0] = %+v", targets[0])
	}
	if targets[1].Up || targets[1].LastError != "connection refused" {
		t.Errorf("target[1] = %+v", targets[1])
	}
	if targets[0].LastScrape.IsZero() {
		t.Error("lastScrape time not parsed")
	}
}
