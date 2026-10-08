package dashboard

import "time"

type DeviceStatus struct {
	DeviceID    string    `json:"device_id"`
	LearnerID   string    `json:"learner_id"`
	CourseID    string    `json:"course_id"`
	Online      bool      `json:"online"`
	ProgressPct int       `json:"progress_pct"`
	Deadline    time.Time `json:"deadline"`
	ObservedAt  time.Time `json:"observed_at"`
}

type DeliveryDecision struct {
	State             string `json:"state"`
	EducatorAttention bool   `json:"educator_attention"`
	Reason            string `json:"reason"`
}

func AssessDelivery(status DeviceStatus, now time.Time) DeliveryDecision {
	if !status.Online {
		return DeliveryDecision{"offline", true, "device_offline"}
	}
	if status.ProgressPct < 100 && !status.Deadline.After(now.Add(24*time.Hour)) {
		return DeliveryDecision{"deadline_risk", true, "incomplete_within_24h"}
	}
	if status.ProgressPct == 100 {
		return DeliveryDecision{"complete", false, "course_complete"}
	}
	return DeliveryDecision{"on_track", false, "active_before_deadline"}
}
