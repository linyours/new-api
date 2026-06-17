package common

import "testing"

func TestMessageWithRequestId(t *testing.T) {
	tests := []struct {
		name    string
		message string
		id      string
		want    string
	}{
		{
			name:    "append current id when no existing suffix",
			message: "bad response status code 400",
			id:      "local-1",
			want:    "bad response status code 400 (request id: local-1)",
		},
		{
			name:    "replace one existing request id suffix",
			message: "bad response status code 400 (request id: upstream-1)",
			id:      "local-1",
			want:    "bad response status code 400 (request id: local-1)",
		},
		{
			name:    "replace multiple existing request id suffixes",
			message: "bad response status code 400 (request id: upstream-1) (request id: upstream-2) (request id: upstream-3)",
			id:      "local-1",
			want:    "bad response status code 400 (request id: local-1)",
		},
		{
			name:    "request id matching is case insensitive",
			message: "bad response status code 400 (Request ID: upstream-1)",
			id:      "local-1",
			want:    "bad response status code 400 (request id: local-1)",
		},
		{
			name:    "return cleaned message when local id is empty",
			message: "bad response status code 400 (request id: upstream-1)",
			id:      "",
			want:    "bad response status code 400",
		},
		{
			name:    "allow message to be empty",
			message: "",
			id:      "local-1",
			want:    "(request id: local-1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MessageWithRequestId(tt.message, tt.id)
			if got != tt.want {
				t.Fatalf("MessageWithRequestId() = %q, want %q", got, tt.want)
			}
		})
	}
}
