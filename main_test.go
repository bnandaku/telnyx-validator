package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	telnyx "github.com/bnandaku/telnyx-api"
)

// fakeLookup sets up a mock Telnyx API server and returns a configured client.
func fakeLookup(t *testing.T, responseBody string, status int) *telnyx.Client {
	t.Helper()
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(mock.Close)
	return telnyx.NewClient("test-key", telnyx.WithBaseURL(mock.URL))
}

func doValidate(t *testing.T, s *server, method, phone string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if method == http.MethodPost {
		body, _ := json.Marshal(map[string]string{"phone_number": phone})
		req = httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(http.MethodGet, "/validate?phone_number="+phone, nil)
	}
	w := httptest.NewRecorder()
	s.handleValidate(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) ValidationResponse {
	t.Helper()
	var resp ValidationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

const mobileResponse = `{
  "data": {
    "record_type": "number_lookup",
    "phone_number": "+15550001111",
    "national_format": "555-000-1111",
    "country_code": "US",
    "carrier": {"name": "AT&T", "type": "mobile", "normalized_carrier": "AT&T"},
    "caller_name": {"caller_name": "John Doe"},
    "portability": {"lrn": "15550001111", "ported_status": "N", "ocn": "6529", "city": "New York", "state": "NY"}
  }
}`

const voipResponse = `{
  "data": {
    "record_type": "number_lookup",
    "phone_number": "+15550002222",
    "national_format": "555-000-2222",
    "country_code": "US",
    "carrier": {"name": "Twilio", "type": "voip", "normalized_carrier": "Twilio"},
    "portability": {"lrn": "15550002222", "ported_status": "N"}
  }
}`

const noCarrierResponse = `{
  "data": {
    "record_type": "number_lookup",
    "phone_number": "+15550003333",
    "national_format": "555-000-3333",
    "country_code": "US",
    "carrier": {"name": "", "type": ""}
  }
}`

func TestValidMobileNumber(t *testing.T) {
	s := &server{client: fakeLookup(t, mobileResponse, 200)}
	w := doValidate(t, s, http.MethodPost, "+15550001111")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	resp := decode(t, w)
	if !resp.Valid {
		t.Errorf("expected valid=true, rejection=%q", resp.RejectionReason)
	}
	if resp.Carrier == nil || resp.Carrier.Name != "AT&T" {
		t.Errorf("expected carrier AT&T, got %+v", resp.Carrier)
	}
	if resp.CallerName == nil || resp.CallerName.CallerName != "John Doe" {
		t.Errorf("expected caller name John Doe, got %+v", resp.CallerName)
	}
}

func TestVoIPRejected(t *testing.T) {
	s := &server{client: fakeLookup(t, voipResponse, 200)}
	w := doValidate(t, s, http.MethodPost, "+15550002222")

	resp := decode(t, w)
	if resp.Valid {
		t.Error("expected valid=false for VoIP number")
	}
	if resp.RejectionReason != "voip_number" {
		t.Errorf("expected rejection_reason=voip_number, got %q", resp.RejectionReason)
	}
}

func TestNoCarrierRejected(t *testing.T) {
	s := &server{client: fakeLookup(t, noCarrierResponse, 200)}
	w := doValidate(t, s, http.MethodGet, "+15550003333")

	resp := decode(t, w)
	if resp.Valid {
		t.Error("expected valid=false for no-carrier number")
	}
	if resp.RejectionReason != "no_carrier" {
		t.Errorf("expected rejection_reason=no_carrier, got %q", resp.RejectionReason)
	}
}

func TestInvalidFormat(t *testing.T) {
	s := &server{client: fakeLookup(t, "{}", 200)}

	cases := []string{"abc", "123", "+"}
	for _, phone := range cases {
		w := doValidate(t, s, http.MethodPost, phone)
		resp := decode(t, w)
		if resp.Valid {
			t.Errorf("phone %q should be invalid", phone)
		}
		if resp.RejectionReason != "invalid_format" {
			t.Errorf("phone %q: expected invalid_format, got %q", phone, resp.RejectionReason)
		}
	}

	// Empty phone_number is rejected at the required-field layer, not format.
	w := doValidate(t, s, http.MethodPost, "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty phone_number: expected 400, got %d", w.Code)
	}
}

func TestNormalisation(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"15550001111", "+15550001111"},
		{"+1 555 000 1111", "+15550001111"},
		{"(555) 000-1111", "+5550001111"},
		{"+1-555-000-1111", "+15550001111"},
	}
	for _, tc := range cases {
		got := normalise(tc.input)
		if got != tc.want {
			t.Errorf("normalise(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handleHealth(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	s := &server{client: fakeLookup(t, "{}", 200)}
	req := httptest.NewRequest(http.MethodPut, "/validate", nil)
	w := httptest.NewRecorder()
	s.handleValidate(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
