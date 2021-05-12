package example

import (
	"context"
	"github.com/Kotodian/cron"
	"testing"
	"time"
)

func TestCron(t *testing.T) {
	err := cron.Every(1).Second().Do(func() {
		println("Hello World!")
	})
	if err != nil {
		t.Error(err)
	}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	_ = cron.Start()
	for {
		select {
		case <-ctx.Done():
			return
		}
	}
}
