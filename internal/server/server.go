package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"alert-dispatcher/internal/adapter"
	"alert-dispatcher/internal/config"
	"alert-dispatcher/notifier"
)

type Server struct {
	signingSecret string
	port          string
	config        *config.Config
}

type SlackPayload struct {
	Type    string `json:"type"`
	Actions []struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value"`
	} `json:"actions"`
	User struct {
		Name string `json:"name"`
	} `json:"user"`
	ResponseURL string `json:"response_url"`
	Message     struct {
		Text   string `json:"text"`
		Blocks []struct {
			Type string `json:"type"`
			Text struct {
				Text string `json:"text"`
			} `json:"text"`
		} `json:"blocks"`
	} `json:"message"`
}

func NewServer(signingSecret, port string, cfg *config.Config) *Server {
	return &Server{
		signingSecret: signingSecret,
		port:          port,
		config:        cfg,
	}
}

func (s *Server) Start() error {
	http.HandleFunc("/slack/events", s.handleInteractive)
	http.HandleFunc("/grafana/webhook", s.handleGrafanaWebhook)
	http.HandleFunc("/health", s.healthCheck)
	log.Printf("Server starting on port %s", s.port)
	return http.ListenAndServe(":"+s.port, nil)
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleInteractive(w http.ResponseWriter, r *http.Request) {
	log.Printf("=== New Request ===")
	log.Printf("Method: %s", r.Method)
	log.Printf("URL: %s", r.URL.Path)
	log.Printf("Content-Type: %s", r.Header.Get("Content-Type"))
	log.Printf("User-Agent: %s", r.Header.Get("User-Agent"))
	log.Printf("X-Slack-Signature: %s", r.Header.Get("X-Slack-Signature"))
	log.Printf("X-Slack-Request-Timestamp: %s", r.Header.Get("X-Slack-Request-Timestamp"))

	if r.Method != http.MethodPost {
		log.Printf("Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	log.Printf("Raw body length: %d", len(body))
	log.Printf("Raw body: %s", string(body))

	// Verify Slack request signature
	if !s.verifySlackRequest(r, body) {
		log.Printf("Slack request verification failed")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	log.Printf("Signature verification: PASSED")

	// Parse URL-encoded form data
	formData, err := url.ParseQuery(string(body))
	if err != nil {
		log.Printf("Failed to parse form data: %v", err)
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	log.Printf("Form data keys: %v", getKeys(formData))

	payloadStr := formData.Get("payload")
	if payloadStr == "" {
		log.Printf("No 'payload' field found in form data")
		http.Error(w, "No payload found", http.StatusBadRequest)
		return
	}

	log.Printf("Extracted payload: %s", payloadStr)

	// Parse the JSON payload
	var slackPayload SlackPayload
	if err := json.Unmarshal([]byte(payloadStr), &slackPayload); err != nil {
		log.Printf("Failed to unmarshal JSON payload: %v", err)
		log.Printf("Payload that failed to parse: %s", payloadStr)
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	log.Printf("Parsed payload type: %s", slackPayload.Type)
	log.Printf("Number of actions: %d", len(slackPayload.Actions))

	if len(slackPayload.Actions) == 0 {
		log.Printf("No actions found in payload")
		http.Error(w, "No actions found", http.StatusBadRequest)
		return
	}

	action := slackPayload.Actions[0]
	alertID := action.Value
	actionType := action.ActionID
	user := slackPayload.User.Name

	log.Printf("Action: %s, Value: %s, User: %s", actionType, alertID, user)

	// Extract alert details from the original message
	alertInfo := s.extractAlertInfo(slackPayload.Message.Text)
	if alertInfo.Name == "" {
		// Fallback: try to extract from blocks
		for _, block := range slackPayload.Message.Blocks {
			if block.Type == "section" {
				alertInfo = s.extractAlertInfo(block.Text.Text)
				if alertInfo.Name != "" {
					break
				}
			}
		}
	}

	var responseText string
	switch actionType {
	case "acknowledge":
		if alertInfo.Name != "" {
			responseText = fmt.Sprintf("✅ **Alert '%s' acknowledged by %s**", alertInfo.Name, user)
			if alertInfo.Description != "" {
				responseText += fmt.Sprintf("\n• *Description:* %s", alertInfo.Description)
			}
			responseText += "\n\n_This alert is now being handled._"
		} else {
			responseText = fmt.Sprintf("✅ **Alert %s acknowledged by %s**\n\n_This alert is now being handled._", alertID, user)
		}
		log.Printf("Alert %s (%s) acknowledged by %s", alertID, alertInfo.Name, user)
	case "dismiss":
		if alertInfo.Name != "" {
			responseText = fmt.Sprintf("❌ **Alert '%s' dismissed by %s**", alertInfo.Name, user)
			if alertInfo.Description != "" {
				responseText += fmt.Sprintf("\n• *Description:* %s", alertInfo.Description)
			}
			responseText += "\n\n_This alert has been dismissed and will not be actioned._"
		} else {
			responseText = fmt.Sprintf("❌ **Alert %s dismissed by %s**\n\n_This alert has been dismissed and will not be actioned._", alertID, user)
		}
		log.Printf("Alert %s (%s) dismissed by %s", alertID, alertInfo.Name, user)
	default:
		responseText = fmt.Sprintf("Unknown action: %s", actionType)
		log.Printf("Unknown action: %s", actionType)
	}

	response := map[string]interface{}{
		"text":             responseText,
		"replace_original": true,
		"response_type":    "in_channel",
	}

	// Send response to Slack via response_url
	if err := s.sendSlackResponse(slackPayload.ResponseURL, response); err != nil {
		log.Printf("Failed to send response to Slack: %v", err)
		http.Error(w, "Failed to send response to Slack", http.StatusInternalServerError)
		return
	}

	// Also send a simple acknowledgment back to the webhook
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	log.Printf("Response sent successfully to Slack")
}

func (s *Server) verifySlackRequest(r *http.Request, body []byte) bool {
	timestamp := r.Header.Get("X-Slack-Request-Timestamp")
	signature := r.Header.Get("X-Slack-Signature")

	log.Printf("Verifying signature - timestamp: %s, signature: %s", timestamp, signature)

	if timestamp == "" || signature == "" {
		log.Printf("Missing timestamp or signature headers")
		return false
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		log.Printf("Failed to parse timestamp: %v", err)
		return false
	}

	timeDiff := time.Now().Unix() - ts
	log.Printf("Time difference: %d seconds", timeDiff)
	if timeDiff > 300 {
		log.Printf("Request too old: %d seconds", timeDiff)
		return false
	}

	baseString := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	h := hmac.New(sha256.New, []byte(s.signingSecret))
	h.Write([]byte(baseString))
	expectedSignature := "v0=" + hex.EncodeToString(h.Sum(nil))

	log.Printf("Expected signature: %s", expectedSignature)
	log.Printf("Received signature: %s", signature)

	isValid := hmac.Equal([]byte(signature), []byte(expectedSignature))
	log.Printf("Signature valid: %t", isValid)
	return isValid
}

// Helper function to get map keys for logging
func getKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// AlertInfo holds extracted alert information
type AlertInfo struct {
	Name        string
	Description string
}

// Extract alarm name from CloudWatch alarm message (legacy function for compatibility)
func (s *Server) extractAlarmName(text string) string {
	info := s.extractAlertInfo(text)
	return info.Name
}

// Extract alert information from alert message (works for both CloudWatch and Grafana)
func (s *Server) extractAlertInfo(text string) AlertInfo {
	var info AlertInfo
	
	// Check for Grafana Alert pattern first
	if strings.Contains(text, "Grafana Alert:") {
		// Extract alert name using regex
		re := regexp.MustCompile(`Grafana Alert: ([^*\n]+)`)
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			info.Name = strings.TrimSpace(matches[1])
		}
		
		// Extract description
		descRe := regexp.MustCompile(`• \*Description:\* ([^\n]+)`)
		descMatches := descRe.FindStringSubmatch(text)
		if len(descMatches) > 1 {
			info.Description = strings.TrimSpace(descMatches[1])
		}
	} else if strings.Contains(text, "CloudWatch Alarm:") {
		// Extract CloudWatch alarm name using regex
		re := regexp.MustCompile(`CloudWatch Alarm: ([^*\n]+)`)
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			info.Name = strings.TrimSpace(matches[1])
		}
		
		// Extract CloudWatch description/reason
		reasonRe := regexp.MustCompile(`• \*Reason:\* ([^\n]+)`)
		reasonMatches := reasonRe.FindStringSubmatch(text)
		if len(reasonMatches) > 1 {
			info.Description = strings.TrimSpace(reasonMatches[1])
		}
	}
	
	return info
}

// Send response to Slack via response_url
func (s *Server) sendSlackResponse(responseURL string, response map[string]interface{}) error {
	payload, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %v", err)
	}

	resp, err := http.Post(responseURL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to post to response_url: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack responded with status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("Successfully sent response to Slack via response_url")
	return nil
}

func (s *Server) handleGrafanaWebhook(w http.ResponseWriter, r *http.Request) {
	log.Printf("=== Grafana Webhook Request ===")
	log.Printf("Method: %s", r.Method)
	log.Printf("URL: %s", r.URL.Path)
	log.Printf("Content-Type: %s", r.Header.Get("Content-Type"))

	if r.Method != http.MethodPost {
		log.Printf("Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	log.Printf("Grafana webhook body: %s", string(body))

	// Process the Grafana alert
	alertMsg, err := adapter.AdaptGrafanaWebhook(string(body), s.config.SlackChannels, s.config.AlarmChannels)
	if err != nil {
		log.Printf("Failed to adapt Grafana webhook: %v", err)
		http.Error(w, "Failed to process alert", http.StatusBadRequest)
		return
	}

	// Create notifier for specific channel
	channelNotifier := notifier.NewSlackNotifier(s.config.SlackBotToken, alertMsg.Channel)
	log.Printf("Sending %s Grafana alert to %s", alertMsg.Priority, alertMsg.Channel)

	// Send to Slack with interactive buttons
	if err := channelNotifier.NotifyWithButtons(alertMsg.Message, fmt.Sprintf("grafana_%d", time.Now().Unix())); err != nil {
		log.Printf("Failed to send Grafana alert to Slack: %v", err)
		http.Error(w, "Failed to send to Slack", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "processed"})

	log.Printf("Grafana webhook processed and sent to Slack successfully")
}
