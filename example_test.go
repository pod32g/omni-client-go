package omni_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	omni "github.com/pod32g/omni-client-go"
)

func ExampleClient_Query() {
	// A real program points New at its server, e.g.
	//   c, _ := omni.New("http://localhost:9090")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[`+
			`{"metric":{"job":"omni"},"value":[1700000000,"1"]},`+
			`{"metric":{"job":"node"},"value":[1700000000,"0"]}]}}`)
	}))
	defer srv.Close()

	c, _ := omni.New(srv.URL)
	res, err := c.Query(context.Background(), "up", time.Time{})
	if err != nil {
		panic(err)
	}
	for _, s := range res.Vector {
		fmt.Printf("up{job=%q} = %g\n", s.Metric["job"], s.Value)
	}
	// Output:
	// up{job="omni"} = 1
	// up{job="node"} = 0
}

func ExampleClient_Targets() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"success","data":[`+
			`{"job":"omni","instance":"127.0.0.1:9090","up":true},`+
			`{"job":"redis","instance":"cache-02:9121","up":false,"lastError":"connection refused"}]}`)
	}))
	defer srv.Close()

	c, _ := omni.New(srv.URL)
	targets, _ := c.Targets(context.Background())
	for _, t := range targets {
		state := "up"
		if !t.Up {
			state = "DOWN: " + t.LastError
		}
		fmt.Printf("%s/%s %s\n", t.Job, t.Instance, state)
	}
	// Output:
	// omni/127.0.0.1:9090 up
	// redis/cache-02:9121 DOWN: connection refused
}
