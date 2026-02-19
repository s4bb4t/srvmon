package check

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func TestReady(t *testing.T) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer cancel()

	ticker := time.NewTicker(time.Millisecond * 300)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			resp, err := http.Get("http://localhost:8080/ready")
			if err != nil {
				continue
			}

			m := make(map[string]any)
			if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
				t.Fatal(err)
			}

			fmt.Println(m)
		case <-ctx.Done():
			return
		}
	}
}
