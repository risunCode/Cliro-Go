package platform

import "testing"

func TestJoinProxyBaseURL_PreventsDoubleV1(t *testing.T) {
	joined := JoinProxyBaseURL("http://127.0.0.1:8095/v1", "/v1/responses")
	if joined != "http://127.0.0.1:8095/v1/responses" {
		t.Fatalf("joined = %q", joined)
	}

	joined = JoinProxyBaseURL("http://127.0.0.1:8095", "/v1/models")
	if joined != "http://127.0.0.1:8095/v1/models" {
		t.Fatalf("joined = %q", joined)
	}
}
