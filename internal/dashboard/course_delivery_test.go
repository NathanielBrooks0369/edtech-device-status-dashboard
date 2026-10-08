package dashboard

import (
	"testing"
	"time"
)

func TestAssessDelivery(t *testing.T) {
	now := time.Date(2026, 9, 26, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		status    DeviceStatus
		wantState string
		wantAlert bool
	}{
		{"offline device", DeviceStatus{Online: false, ProgressPct: 80, Deadline: now.Add(72 * time.Hour)}, "offline", true},
		{"deadline approaching", DeviceStatus{Online: true, ProgressPct: 80, Deadline: now.Add(12 * time.Hour)}, "deadline_risk", true},
		{"course complete", DeviceStatus{Online: true, ProgressPct: 100, Deadline: now.Add(2 * time.Hour)}, "complete", false},
		{"learner on track", DeviceStatus{Online: true, ProgressPct: 40, Deadline: now.Add(48 * time.Hour)}, "on_track", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AssessDelivery(tt.status, now)
			if got.State != tt.wantState || got.EducatorAttention != tt.wantAlert {
				t.Fatalf("AssessDelivery() = %#v, want state=%q attention=%t", got, tt.wantState, tt.wantAlert)
			}
		})
	}
}
