package metrics

import (
	"testing"
	"time"
)

func TestActivitySnapshot(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		activity Activity
		state    string
		ageMS    int64
	}{
		{"never received", Activity{}, "never_received", -1},
		{"fresh", Activity{Count: 2, Last: now}, "flowing", 0},
		{"below threshold", Activity{Count: 2, Last: now.Add(-HealthTimeout + time.Millisecond)}, "flowing", 4999},
		{"at threshold", Activity{Count: 2, Last: now.Add(-HealthTimeout)}, "stalled", 5000},
		{"future observation", Activity{Count: 2, Last: now.Add(time.Second)}, "flowing", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.activity.Snapshot(now)
			if got.State != tt.state || got.Count != tt.activity.Count {
				t.Fatalf("snapshot = %+v, want %s count %d", got, tt.state, tt.activity.Count)
			}
			if tt.ageMS < 0 {
				if got.AgeMS != nil {
					t.Fatal("never-received activity must not have an age")
				}
			} else if got.AgeMS == nil || *got.AgeMS != tt.ageMS {
				t.Fatalf("age = %v, want %d", got.AgeMS, tt.ageMS)
			}
		})
	}
}
