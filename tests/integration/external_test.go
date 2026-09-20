//go:build integration

package integration

import "testing"

// TestExternalTools verifies that external management UIs are reachable.
func TestExternalTools(t *testing.T) {
	t.Run("RabbitMQ_Management", func(t *testing.T) {
		code, _ := apiGet(t, rabbitmqMgmtURL+"/")
		// RabbitMQ management returns 200 or 301 (redirect to login).
		if code != 200 && code != 301 {
			t.Errorf("RabbitMQ Management: got %d, want 200 or 301", code)
		}
	})

	t.Run("Kafdrop", func(t *testing.T) {
		code, _ := apiGet(t, kafdropURL+"/")
		// Kafdrop returns 200 or 302 (redirect).
		if code != 200 && code != 302 {
			t.Errorf("Kafdrop: got %d, want 200 or 302", code)
		}
	})
}
