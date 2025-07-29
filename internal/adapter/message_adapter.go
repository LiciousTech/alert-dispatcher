package adapter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type CloudWatchAlarm struct {
	AlarmName        string  `json:"AlarmName"`
	AlarmDescription *string `json:"AlarmDescription"`
	AWSAccountId     string  `json:"AWSAccountId"`
	NewStateValue    string  `json:"NewStateValue"`
	OldStateValue    string  `json:"OldStateValue"`
	NewStateReason   string  `json:"NewStateReason"`
	StateChangeTime  string  `json:"StateChangeTime"`
	Region           string  `json:"Region"`
	AlarmArn         string  `json:"AlarmArn"`
	Trigger          struct {
		MetricName         string  `json:"MetricName"`
		Namespace          string  `json:"Namespace"`
		Statistic          string  `json:"Statistic"`
		ComparisonOperator string  `json:"ComparisonOperator"`
		Threshold          float64 `json:"Threshold"`
		Period             int     `json:"Period"`
		EvaluationPeriods  int     `json:"EvaluationPeriods"`
		Dimensions         []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"Dimensions"`
	} `json:"Trigger"`
}

type GrafanaAlert struct {
	Title       string            `json:"title"`
	RuleID      int64             `json:"ruleId"`
	RuleName    string            `json:"ruleName"`
	State       string            `json:"state"`
	EvalMatches []EvalMatch       `json:"evalMatches"`
	OrgID       int64             `json:"orgId"`
	DashboardID int64             `json:"dashboardId"`
	PanelID     int64             `json:"panelId"`
	Tags        map[string]string `json:"tags"`
	RuleURL     string            `json:"ruleUrl"`
	Message     string            `json:"message"`
}

type EvalMatch struct {
	Value  float64           `json:"value"`
	Metric string            `json:"metric"`
	Tags   map[string]string `json:"tags"`
}

type GrafanaWebhook struct {
	DashboardID int64             `json:"dashboardId"`
	EvalMatches []EvalMatch       `json:"evalMatches"`
	ImageURL    string            `json:"imageUrl"`
	Message     string            `json:"message"`
	OrgID       int64             `json:"orgId"`
	PanelID     int64             `json:"panelId"`
	RuleID      int64             `json:"ruleId"`
	RuleName    string            `json:"ruleName"`
	RuleURL     string            `json:"ruleUrl"`
	State       string            `json:"state"`
	Tags        map[string]string `json:"tags"`
	Title       string            `json:"title"`
}

type AlertMessage struct {
	Message  string
	Priority string
	Channel  string
}

func AdaptSQSMessage(body string) (string, error) {
	var envelope struct {
		Message string `json:"Message"`
		Subject string `json:"Subject"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return "", err
	}

	var alarm CloudWatchAlarm
	if err := json.Unmarshal([]byte(envelope.Message), &alarm); err != nil {
		return "", err
	}

	return formatSlackMessage(alarm), nil
}

func AdaptSQSMessageWithRouting(body string, channels map[string]string, alarmChannels map[string]string) (*AlertMessage, error) {
	var envelope struct {
		Message string `json:"Message"`
		Subject string `json:"Subject"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return nil, err
	}

	var alarm CloudWatchAlarm
	if err := json.Unmarshal([]byte(envelope.Message), &alarm); err != nil {
		return nil, err
	}

	// First check if there's a specific mapping for this alarm
	channel := alarmChannels[alarm.AlarmName]

	// If no specific mapping, use priority-based routing
	if channel == "" {
		priority := determinePriority(alarm)
		channel = channels[priority]
		if channel == "" {
			channel = channels["default"]
		}
	}

	priority := determinePriority(alarm)

	return &AlertMessage{
		Message:  formatSlackMessage(alarm),
		Priority: priority,
		Channel:  channel,
	}, nil
}

// This will be rarely used as this is just a fallback if mapping is not done via configmap
func determinePriority(alarm CloudWatchAlarm) string {
	// Priority logic - customize based on your needs
	alarmName := strings.ToLower(alarm.AlarmName)
	namespace := alarm.Trigger.Namespace

	// P0 - Critical production services
	if strings.Contains(alarmName, "prod") || strings.Contains(alarmName, "production") {
		return "P0"
	}

	// P0 - Database and critical infrastructure
	if strings.Contains(namespace, "RDS") || strings.Contains(namespace, "DynamoDB") {
		return "P0"
	}

	if strings.Contains(namespace, "ELB") || strings.Contains(namespace, "5xx") {
		return "P0"
	}

	// P0 - High CPU/Memory alerts
	if strings.Contains(alarmName, "cpu") || strings.Contains(alarmName, "memory") {
		return "P0"
	}

	if strings.Contains(alarmName, "redis") || strings.Contains(alarmName, "elasticache") {
		return "P1"
	}

	if strings.Contains(alarmName, "qa") || strings.Contains(alarmName, "staging") {
		return "P2"
	}

	// P2 - Everything else
	return "P2"
}

func formatSlackMessage(alarm CloudWatchAlarm) string {
	// Get emoji and color based on state
	var emoji, stateColor string
	switch alarm.NewStateValue {
	case "ALARM":
		emoji = "🚨"
		stateColor = "`🔴 ALARM`"
	case "OK":
		emoji = "✅"
		stateColor = "`🟢 OK`"
	case "INSUFFICIENT_DATA":
		emoji = "⚠️"
		stateColor = "`🟡 INSUFFICIENT_DATA`"
	default:
		emoji = "📊"
		stateColor = fmt.Sprintf("`%s`", alarm.NewStateValue)
	}

	var oldStateColor string
	switch alarm.OldStateValue {
	case "ALARM":
		oldStateColor = "`🔴 ALARM`"
	case "OK":
		oldStateColor = "`🟢 OK`"
	case "INSUFFICIENT_DATA":
		oldStateColor = "`🟡 INSUFFICIENT_DATA`"
	default:
		oldStateColor = fmt.Sprintf("`%s`", alarm.OldStateValue)
	}

	// Build the message with color coding
	message := fmt.Sprintf(`%s *CloudWatch Alarm: %s*
• *From:* %s → *To:* %s
• *Metric:* `+"`%s/%s`"+`
• *Threshold:* `+"`%s %.1f`"+`
• *Period:* `+"`%ds over %d evaluations`"+`
• *Dimensions:*
%s
• *Region:* `+"`%s`"+`
• *Reason:* %s
• *Time:* `+"`%s`",
		emoji, alarm.AlarmName,
		oldStateColor, stateColor,
		alarm.Trigger.Namespace, alarm.Trigger.MetricName,
		alarm.Trigger.ComparisonOperator, alarm.Trigger.Threshold,
		alarm.Trigger.Period, alarm.Trigger.EvaluationPeriods,
		formatDimensionsIndented(alarm.Trigger.Dimensions),
		alarm.Region,
		alarm.NewStateReason,
		formatTimestamp(alarm.StateChangeTime))

	return message
}

func formatDimensionsIndented(dimensions []struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}) string {
	if len(dimensions) == 0 {
		return "   → None"
	}

	var parts []string
	for _, dim := range dimensions {
		parts = append(parts, fmt.Sprintf("   → %s: %s", dim.Name, dim.Value))
	}
	return strings.Join(parts, "\n")
}

func formatTimestamp(timeStr string) string {
	// CloudWatch sends timestamps in format: "2025-07-23T13:32:26.882+0000"
	// Parse it and format for display
	t, err := time.Parse("2006-01-02T15:04:05.000+0000", timeStr)
	if err != nil {
		// If parsing fails, return the original string
		return timeStr
	}
	return t.Format("2006-01-02 15:04:05 UTC")
}

func AdaptGrafanaWebhook(body string, channels map[string]string, alarmChannels map[string]string) (*AlertMessage, error) {
	// First try modern Alertmanager format
	var alertmanagerWebhook struct {
		Alerts       []map[string]interface{} `json:"alerts"`
		CommonLabels map[string]string        `json:"commonLabels"`
		Status       string                   `json:"status"`
		Title        string                   `json:"title"`
		Message      string                   `json:"message"`
	}

	if err := json.Unmarshal([]byte(body), &alertmanagerWebhook); err == nil && len(alertmanagerWebhook.Alerts) > 0 {
		return adaptAlertmanagerWebhook(alertmanagerWebhook, channels, alarmChannels)
	}

	// Fallback to legacy format
	var grafanaAlert GrafanaWebhook
	if err := json.Unmarshal([]byte(body), &grafanaAlert); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Grafana webhook: %v", err)
	}

	// First check if there's a specific mapping for this rule
	channel := alarmChannels[grafanaAlert.RuleName]

	// If no specific mapping, use priority-based routing
	if channel == "" {
		priority := determineGrafanaPriority(grafanaAlert)
		channel = channels[priority]
		if channel == "" {
			channel = channels["default"]
		}
	}

	priority := determineGrafanaPriority(grafanaAlert)

	return &AlertMessage{
		Message:  formatGrafanaSlackMessage(grafanaAlert),
		Priority: priority,
		Channel:  channel,
	}, nil
}

func adaptAlertmanagerWebhook(webhook struct {
	Alerts       []map[string]interface{} `json:"alerts"`
	CommonLabels map[string]string        `json:"commonLabels"`
	Status       string                   `json:"status"`
	Title        string                   `json:"title"`
	Message      string                   `json:"message"`
}, channels map[string]string, alarmChannels map[string]string) (*AlertMessage, error) {

	// Get channel from commonLabels first
	var channelTag string
	if webhook.CommonLabels != nil {
		channelTag = webhook.CommonLabels["channel"]
	}

	// If no channel in commonLabels, check first alert
	if channelTag == "" && len(webhook.Alerts) > 0 {
		if labels, ok := webhook.Alerts[0]["labels"].(map[string]interface{}); ok {
			if ch, exists := labels["channel"]; exists {
				if chStr, ok := ch.(string); ok {
					channelTag = chStr
				}
			}
		}
	}

	// Debug log to see what channel tag was extracted
	fmt.Printf("DEBUG: Extracted channelTag: '%s'\n", channelTag)

	// Determine priority from channel tag or fallback logic
	var priority string

	// Check for NoData state and route to P1
	if strings.ToUpper(webhook.Status) == "FIRING" && len(webhook.Alerts) > 0 {
		// Check if any alert is in NoData state
		for _, alert := range webhook.Alerts {
			if labels, ok := alert["labels"].(map[string]interface{}); ok {
				if alertname, exists := labels["alertname"]; exists {
					if alertnameStr, ok := alertname.(string); ok {
						// Check for NoData indicators in alert name or labels
						if strings.Contains(strings.ToLower(alertnameStr), "nodata") ||
							strings.Contains(strings.ToLower(alertnameStr), "no data") ||
							strings.Contains(strings.ToLower(alertnameStr), "data source") {
							priority = "P1"
							fmt.Printf("DEBUG: NoData alert detected, setting priority to P1: %s\n", alertnameStr)
							break
						}
					}
				}
			}

			// Also check annotations for NoData indicators
			if annotations, ok := alert["annotations"].(map[string]interface{}); ok {
				if description, exists := annotations["description"]; exists {
					if descStr, ok := description.(string); ok {
						if strings.Contains(strings.ToLower(descStr), "no data") ||
							strings.Contains(strings.ToLower(descStr), "nodata") ||
							strings.Contains(strings.ToLower(descStr), "data source") {
							priority = "P1"
							fmt.Printf("DEBUG: NoData alert detected in description, setting priority to P1\n")
							break
						}
					}
				}
			}
		}
	}

	// If priority not set by NoData logic, use channel tag or fallback
	if priority == "" {
		if channelTag != "" {
			switch strings.ToUpper(channelTag) {
			case "P0":
				priority = "P0"
			case "P1":
				priority = "P1"
			case "P2":
				priority = "P2"
			default:
				priority = "P2"
			}
		} else {
			priority = "P2" // default
		}
	}

	// Debug log to see final priority
	fmt.Printf("DEBUG: Final priority: '%s'\n", priority)

	// Get alertname for specific mapping check
	alertname := ""
	if webhook.CommonLabels != nil {
		alertname = webhook.CommonLabels["alertname"]
	}

	// First check if there's a specific mapping for this alert
	channel := alarmChannels[alertname]

	// If no specific mapping, use priority-based routing
	if channel == "" {
		channel = channels[priority]
		if channel == "" {
			channel = channels["default"]
		}
	}

	return &AlertMessage{
		Message:  formatAlertmanagerSlackMessage(webhook),
		Priority: priority,
		Channel:  channel,
	}, nil
}

func determineGrafanaPriority(alert GrafanaWebhook) string {
	// First check if there's an explicit channel tag
	if channelTag, exists := alert.Tags["channel"]; exists {
		switch strings.ToUpper(channelTag) {
		case "P0":
			return "P0"
		case "P1":
			return "P1"
		case "P2":
			return "P2"
		}
	}

	// Fallback to heuristic-based priority logic
	ruleName := strings.ToLower(alert.RuleName)
	title := strings.ToLower(alert.Title)

	// P0 - Critical alerts
	if strings.Contains(ruleName, "critical") || strings.Contains(title, "critical") {
		return "P0"
	}
	if strings.Contains(ruleName, "prod") || strings.Contains(title, "prod") {
		return "P0"
	}
	if strings.Contains(ruleName, "down") || strings.Contains(title, "down") {
		return "P0"
	}

	// P1 - High priority
	if strings.Contains(ruleName, "high") || strings.Contains(title, "high") {
		return "P1"
	}
	if strings.Contains(ruleName, "error") || strings.Contains(title, "error") {
		return "P1"
	}

	// P2 - Medium/Low priority
	if strings.Contains(ruleName, "warning") || strings.Contains(title, "warning") {
		return "P2"
	}
	if strings.Contains(ruleName, "staging") || strings.Contains(title, "staging") {
		return "P2"
	}

	// Default to P2
	return "P2"
}

func formatGrafanaSlackMessage(alert GrafanaWebhook) string {
	// Get emoji and color based on state
	var emoji, stateColor string
	switch strings.ToUpper(alert.State) {
	case "ALERTING":
		emoji = "🚨"
		stateColor = "`🔴 ALERTING`"
	case "OK":
		emoji = "✅"
		stateColor = "`🟢 OK`"
	case "NO_DATA":
		emoji = "⚠️"
		stateColor = "`🟡 NO_DATA`"
	case "PENDING":
		emoji = "⏳"
		stateColor = "`🟡 PENDING`"
	default:
		emoji = "📊"
		stateColor = fmt.Sprintf("`%s`", alert.State)
	}

	// Build the message with better formatting
	message := fmt.Sprintf(`%s *Grafana Alert: %s*
• *State:* %s`,
		emoji, alert.Title,
		stateColor)

	// Add rule name only if it's different from title and not empty
	if alert.RuleName != "" && alert.RuleName != alert.Title {
		message += fmt.Sprintf("\n• *Rule:* `%s`", alert.RuleName)
	}

	// Add message if not empty
	if alert.Message != "" {
		message += fmt.Sprintf("\n• *Description:* %s", alert.Message)
	}

	// Add evaluation matches with better formatting
	if len(alert.EvalMatches) > 0 {
		message += "\n• *Metrics:*"
		for _, match := range alert.EvalMatches {
			// Format the value nicely
			valueStr := fmt.Sprintf("%.2f", match.Value)
			if match.Value == float64(int64(match.Value)) {
				valueStr = fmt.Sprintf("%.0f", match.Value)
			}
			message += fmt.Sprintf("\n   → `%s`: **%s**", match.Metric, valueStr)

			// Add tags in a cleaner format
			if len(match.Tags) > 0 {
				var importantTags []string
				for k, v := range match.Tags {
					// Only show important tags, skip noise
					if k != "__name__" && k != "job" && k != "instance" {
						importantTags = append(importantTags, fmt.Sprintf("`%s=%s`", k, v))
					}
				}
				if len(importantTags) > 0 {
					message += fmt.Sprintf(" (%s)", strings.Join(importantTags, ", "))
				}
			}
		}
	}

	// Add important tags only (filter out noise)
	if len(alert.Tags) > 0 {
		var importantTags []string
		for k, v := range alert.Tags {
			// Skip channel tag as it's used for routing
			if k != "channel" && v != "" {
				importantTags = append(importantTags, fmt.Sprintf("   → `%s`: %s", k, v))
			}
		}
		if len(importantTags) > 0 {
			message += "\n• *Labels:*\n" + strings.Join(importantTags, "\n")
		}
	}

	// Add rule URL if available
	if alert.RuleURL != "" {
		message += fmt.Sprintf("\n• *Dashboard:* <%s|View Alert Rule>", alert.RuleURL)
	}

	return message
}

func formatAlertmanagerSlackMessage(webhook struct {
	Alerts       []map[string]interface{} `json:"alerts"`
	CommonLabels map[string]string        `json:"commonLabels"`
	Status       string                   `json:"status"`
	Title        string                   `json:"title"`
	Message      string                   `json:"message"`
}) string {
	// Wrap everything in a defer to catch any panics and return basic message
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Error formatting alert message, using fallback: %v\n", r)
		}
	}()

	// Try enhanced formatting first
	if enhancedMessage := formatEnhancedAlertMessage(webhook); enhancedMessage != "" {
		return enhancedMessage
	}

	// Fallback to basic format
	return formatBasicAlertMessage(webhook)
}

func formatEnhancedAlertMessage(webhook struct {
	Alerts       []map[string]interface{} `json:"alerts"`
	CommonLabels map[string]string        `json:"commonLabels"`
	Status       string                   `json:"status"`
	Title        string                   `json:"title"`
	Message      string                   `json:"message"`
}) string {
	// Get emoji and color based on status
	var emoji, stateColor string
	switch strings.ToUpper(webhook.Status) {
	case "FIRING":
		emoji = "🚨"
		stateColor = "`🔴 FIRING`"
	case "RESOLVED":
		emoji = "✅"
		stateColor = "`🟢 RESOLVED`"
	default:
		emoji = "📊"
		stateColor = fmt.Sprintf("`%s`", webhook.Status)
	}

	// Build the message
	alertname := webhook.CommonLabels["alertname"]
	if alertname == "" && len(webhook.Alerts) > 0 {
		if labels, ok := webhook.Alerts[0]["labels"].(map[string]interface{}); ok {
			if name, exists := labels["alertname"]; exists {
				if nameStr, ok := name.(string); ok {
					alertname = nameStr
				}
			}
		}
	}

	message := fmt.Sprintf(`%s *Grafana Alert: %s*
• *State:* %s`,
		emoji, alertname, stateColor)

	// Add annotations from the alert
	if len(webhook.Alerts) > 0 {
		if annotations, ok := webhook.Alerts[0]["annotations"].(map[string]interface{}); ok {
			// Add description/summary first
			if desc, exists := annotations["description"]; exists {
				if descStr, ok := desc.(string); ok && descStr != "" {
					message += fmt.Sprintf("\n• *Description:* %s", descStr)
				}
			} else if summary, exists := annotations["summary"]; exists {
				if summaryStr, ok := summary.(string); ok && summaryStr != "" {
					message += fmt.Sprintf("\n• *Description:* %s", summaryStr)
				}
			}

			// Add all other annotations dynamically
			for key, value := range annotations {
				// Skip already processed annotations
				if key == "description" || key == "summary" {
					continue
				}

				if valueStr, ok := value.(string); ok && valueStr != "" {
					// Format the key nicely (capitalize first letter)
					formattedKey := strings.ReplaceAll(key, "_", " ")
					if len(formattedKey) > 0 {
						formattedKey = strings.ToUpper(formattedKey[:1]) + formattedKey[1:]
					}
					message += fmt.Sprintf("\n• *%s:* %s", formattedKey, valueStr)
				}
			}
		}
	}

	// Add valueString as raw data if available
	if len(webhook.Alerts) > 0 {
		alert := webhook.Alerts[0]
		if valueString, ok := alert["valueString"].(string); ok && valueString != "" {
			message += fmt.Sprintf("\n• *ValueString:* %s", valueString)
		}
	}

	// Add silence URL if available
	if len(webhook.Alerts) > 0 {
		if silenceURL, ok := webhook.Alerts[0]["silenceURL"].(string); ok && silenceURL != "" {
			message += fmt.Sprintf("\n• *Silence:* <%s|Silence Alert>", silenceURL)
		}
	}

	// Add generator URL if available
	if len(webhook.Alerts) > 0 {
		if generatorURL, ok := webhook.Alerts[0]["generatorURL"].(string); ok && generatorURL != "" {
			message += fmt.Sprintf("\n• *Dashboard:* <%s|View Alert Rule>", generatorURL)
		}
	}

	// Add dashboard URL if different from generator URL
	if len(webhook.Alerts) > 0 {
		if dashboardURL, ok := webhook.Alerts[0]["dashboardURL"].(string); ok && dashboardURL != "" {
			// Only add if it's different from generator URL
			generatorURL, _ := webhook.Alerts[0]["generatorURL"].(string)
			if dashboardURL != generatorURL {
				message += fmt.Sprintf("\n• *Dashboard:* <%s|View Dashboard>", dashboardURL)
			}
		}
	}

	return message
}

func formatBasicAlertMessage(webhook struct {
	Alerts       []map[string]interface{} `json:"alerts"`
	CommonLabels map[string]string        `json:"commonLabels"`
	Status       string                   `json:"status"`
	Title        string                   `json:"title"`
	Message      string                   `json:"message"`
}) string {
	// Get emoji and color based on status
	var emoji, stateColor string
	switch strings.ToUpper(webhook.Status) {
	case "FIRING":
		emoji = "🚨"
		stateColor = "`🔴 FIRING`"
	case "RESOLVED":
		emoji = "✅"
		stateColor = "`🟢 RESOLVED`"
	default:
		emoji = "📊"
		stateColor = fmt.Sprintf("`%s`", webhook.Status)
	}

	// Build the basic message
	alertname := webhook.CommonLabels["alertname"]
	if alertname == "" && len(webhook.Alerts) > 0 {
		if labels, ok := webhook.Alerts[0]["labels"].(map[string]interface{}); ok {
			if name, exists := labels["alertname"]; exists {
				if nameStr, ok := name.(string); ok {
					alertname = nameStr
				}
			}
		}
	}

	message := fmt.Sprintf(`%s *Grafana Alert: %s*
• *State:* %s`,
		emoji, alertname, stateColor)

	// Add description/summary from annotations
	if len(webhook.Alerts) > 0 {
		if annotations, ok := webhook.Alerts[0]["annotations"].(map[string]interface{}); ok {
			if desc, exists := annotations["description"]; exists {
				if descStr, ok := desc.(string); ok && descStr != "" {
					message += fmt.Sprintf("\n• *Description:* %s", descStr)
				}
			} else if summary, exists := annotations["summary"]; exists {
				if summaryStr, ok := summary.(string); ok && summaryStr != "" {
					message += fmt.Sprintf("\n• *Description:* %s", summaryStr)
				}
			}
		}
	}

	// Add generator URL if available
	if len(webhook.Alerts) > 0 {
		if generatorURL, ok := webhook.Alerts[0]["generatorURL"].(string); ok && generatorURL != "" {
			message += fmt.Sprintf("\n• *Dashboard:* <%s|View Alert Rule>", generatorURL)
		}
	}

	return message
}

func formatValueString(valueString string) string {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Error parsing valueString, skipping: %v\n", r)
		}
	}()

	var result strings.Builder

	// Split by "], [" to separate individual metrics
	values := strings.Split(valueString, "], [")
	for i, val := range values {
		// Clean up the value string
		val = strings.Trim(val, "[]")
		val = strings.TrimSpace(val)

		// Extract pod name and value for better formatting
		if strings.Contains(val, "labels={pod=") && strings.Contains(val, "} value=") {
			// Extract pod name
			podStart := strings.Index(val, "labels={pod=") + 12
			podEnd := strings.Index(val[podStart:], "}")
			if podEnd > 0 {
				podName := val[podStart : podStart+podEnd]

				// Extract value
				valueStart := strings.Index(val, "} value=") + 8
				valueStr := val[valueStart:]

				// Try to parse and format the value
				if value, err := parseFloat(valueStr); err == nil {
					result.WriteString(fmt.Sprintf("\n   → `%s`: **%.2f%%**", podName, value))
				} else {
					result.WriteString(fmt.Sprintf("\n   → `%s`: **%s**", podName, valueStr))
				}
			}
		} else {
			// Fallback for other value formats
			if i < 5 { // Limit to first 5 values to avoid too long messages
				result.WriteString(fmt.Sprintf("\n   → `%s`", val))
			} else if i == 5 {
				remaining := len(values) - 5
				result.WriteString(fmt.Sprintf("\n   → ... and %d more values", remaining))
				break
			}
		}
	}

	return result.String()
}

// Helper function to extract pod names from valueString
func extractPodNames(valueString string) []string {
	var podNames []string
	seen := make(map[string]bool) // To avoid duplicates

	// Split by "], [" to separate individual metrics
	values := strings.Split(valueString, "], [")
	for _, val := range values {
		// Clean up the value string
		val = strings.Trim(val, "[]")
		val = strings.TrimSpace(val)

		// Extract pod name if present
		if strings.Contains(val, "labels={pod=") {
			podStart := strings.Index(val, "labels={pod=") + 12
			podEnd := strings.Index(val[podStart:], "}")
			if podEnd > 0 {
				podName := val[podStart : podStart+podEnd]
				if !seen[podName] {
					podNames = append(podNames, podName)
					seen[podName] = true
				}
			}
		}
	}

	return podNames
}

// Helper function to parse float values from string
func parseFloat(s string) (float64, error) {
	// Remove any trailing spaces or characters
	s = strings.TrimSpace(s)

	// Try to parse as float
	if val, err := fmt.Sscanf(s, "%f", new(float64)); err == nil && val == 1 {
		var result float64
		fmt.Sscanf(s, "%f", &result)
		return result, nil
	}

	return 0, fmt.Errorf("unable to parse float from: %s", s)
}
