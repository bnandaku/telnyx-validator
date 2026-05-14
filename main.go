package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	telnyx "github.com/bnandaku/telnyx-api"
)

// e164Pattern accepts E.164 numbers: +1–+999 followed by 4–14 digits.
var e164Pattern = regexp.MustCompile(`^\+[1-9]\d{4,14}$`)

// ValidationResponse is returned by POST /validate and GET /validate.
type ValidationResponse struct {
	PhoneNumber     string                    `json:"phone_number"`
	Valid           bool                      `json:"valid"`
	RejectionReason string                    `json:"rejection_reason,omitempty"`
	Carrier         *telnyx.CarrierInfo       `json:"carrier,omitempty"`
	CallerName      *telnyx.CallerNameInfo    `json:"caller_name,omitempty"`
	Portability     *telnyx.PortabilityInfo   `json:"portability,omitempty"`
	NationalFormat  string                    `json:"national_format,omitempty"`
	CountryCode     string                    `json:"country_code,omitempty"`
}

type validateRequest struct {
	PhoneNumber string `json:"phone_number"`
}

func main() {
	apiKey := os.Getenv("TELNYX_API_KEY")
	if apiKey == "" {
		log.Fatal("TELNYX_API_KEY environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	client := telnyx.NewClient(apiKey)
	srv := &server{client: client}

	mux := http.NewServeMux()
	mux.HandleFunc("/validate", srv.handleValidate)
	mux.HandleFunc("/health", handleHealth)

	log.Printf("listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

type server struct {
	client *telnyx.Client
}

func (s *server) handleValidate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleValidateGET(w, r)
	case http.MethodPost:
		s.handleValidatePOST(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *server) handleValidateGET(w http.ResponseWriter, r *http.Request) {
	phone := r.URL.Query().Get("phone_number")
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone_number query parameter is required")
		return
	}
	s.validate(w, r, phone)
}

func (s *server) handleValidatePOST(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.PhoneNumber == "" {
		writeError(w, http.StatusBadRequest, "phone_number is required")
		return
	}
	s.validate(w, r, req.PhoneNumber)
}

func (s *server) validate(w http.ResponseWriter, r *http.Request, phoneNumber string) {
	// Normalise: strip spaces/dashes, ensure E.164 prefix.
	phoneNumber = normalise(phoneNumber)

	if !e164Pattern.MatchString(phoneNumber) {
		writeJSON(w, http.StatusUnprocessableEntity, ValidationResponse{
			PhoneNumber:     phoneNumber,
			Valid:           false,
			RejectionReason: "invalid_format",
		})
		return
	}

	result, err := s.client.LookupNumber(r.Context(), phoneNumber)
	if err != nil {
		var apiErr *telnyx.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			writeJSON(w, http.StatusOK, ValidationResponse{
				PhoneNumber:     phoneNumber,
				Valid:           false,
				RejectionReason: "not_found",
			})
			return
		}
		writeError(w, http.StatusBadGateway, fmt.Sprintf("lookup failed: %s", err))
		return
	}

	resp := ValidationResponse{
		PhoneNumber:    result.PhoneNumber,
		NationalFormat: result.NationalFormat,
		CountryCode:    result.CountryCode,
		Carrier:        result.Carrier,
		CallerName:     result.CallerName,
		Portability:    result.Portability,
	}

	// Reject if no carrier resolved.
	if result.Carrier == nil || result.Carrier.Name == "" {
		resp.Valid = false
		resp.RejectionReason = "no_carrier"
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Reject VoIP numbers.
	if strings.EqualFold(result.Carrier.Type, "voip") {
		resp.Valid = false
		resp.RejectionReason = "voip_number"
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.Valid = true
	writeJSON(w, http.StatusOK, resp)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// normalise strips whitespace and common punctuation, then ensures E.164 format.
func normalise(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "(", "")
	s = strings.ReplaceAll(s, ")", "")
	s = strings.ReplaceAll(s, ".", "")
	if !strings.HasPrefix(s, "+") {
		s = "+" + s
	}
	return s
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
