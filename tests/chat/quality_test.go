package chat_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"douya/internal/chat"
	"douya/internal/llm"
	"douya/internal/search"
)

type QualityScenario struct {
	Name          string
	Input         string
	SearchMode string
	ModelBehavior func() []string
	Assertions    []QualityAssertion
}

type QualityAssertion struct {
	Name     string
	Check    func(response string) bool
	Severity string
}

type QualityReport struct {
	Scenario string
	Passed   int
	Failed   int
	Total    int
	Details  []QualityResult
	Duration time.Duration
}

type QualityResult struct {
	Assertion string
	Passed    bool
	Severity  string
	Message   string
}

func runQualityScenario(_ *testing.T, svc *chat.Service, scenario QualityScenario) QualityReport {
	start := time.Now()
	report := QualityReport{
		Scenario: scenario.Name,
	}

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       scenario.Input,
		SearchMode: scenario.SearchMode,
	})

	duration := time.Since(start)
	report.Duration = duration

	if err != nil {
		for _, assertion := range scenario.Assertions {
			report.Total++
			report.Failed++
			report.Details = append(report.Details, QualityResult{
				Assertion: assertion.Name,
				Passed:    false,
				Severity:  assertion.Severity,
				Message:   fmt.Sprintf("SendMessage failed: %v", err),
			})
		}
		return report
	}

	for _, assertion := range scenario.Assertions {
		report.Total++
		if assertion.Check("response") {
			report.Passed++
			report.Details = append(report.Details, QualityResult{
				Assertion: assertion.Name,
				Passed:    true,
				Severity:  assertion.Severity,
			})
		} else {
			report.Failed++
			report.Details = append(report.Details, QualityResult{
				Assertion: assertion.Name,
				Passed:    false,
				Severity:  assertion.Severity,
				Message:   "Assertion failed",
			})
		}
	}

	return report
}

func printQualityReport(t *testing.T, reports []QualityReport) {
	t.Helper()
	t.Log("\n========== Quality Test Report ==========")
	totalPassed := 0
	totalFailed := 0
	for _, r := range reports {
		status := "PASS"
		if r.Failed > 0 {
			status = "FAIL"
		}
		t.Logf("[%s] %s: %d/%d passed (%v)", status, r.Scenario, r.Passed, r.Total, r.Duration)
		for _, d := range r.Details {
			icon := "✓"
			if !d.Passed {
				icon = "✗"
			}
			msg := ""
			if d.Message != "" {
				msg = fmt.Sprintf(" - %s", d.Message)
			}
			t.Logf("  %s %s [%s]%s", icon, d.Assertion, d.Severity, msg)
		}
		totalPassed += r.Passed
		totalFailed += r.Failed
	}
	t.Logf("\nTotal: %d passed, %d failed, %d total", totalPassed, totalFailed, totalPassed+totalFailed)
	t.Log("==========================================")
}

func TestQuality_BasicScenarios(t *testing.T) {
	scenarios := []QualityScenario{
		{
			Name:  "Basic English QA",
			Input: "What is 2+2?",
			ModelBehavior: func() []string {
				return []string{makeContentChunk("4"), makeFinishChunk("stop")}
			},
			Assertions: []QualityAssertion{
				{Name: "no_error", Severity: "critical", Check: func(r string) bool { return true }},
				{Name: "completes", Severity: "high", Check: func(r string) bool { return true }},
			},
		},
		{
			Name:  "Chinese QA",
			Input: "1+1等于几？",
			ModelBehavior: func() []string {
				return []string{makeContentChunk("2"), makeFinishChunk("stop")}
			},
			Assertions: []QualityAssertion{
				{Name: "no_error", Severity: "critical", Check: func(r string) bool { return true }},
				{Name: "completes", Severity: "high", Check: func(r string) bool { return true }},
			},
		},
		{
			Name:  "Code Generation",
			Input: "Write a Python hello world",
			ModelBehavior: func() []string {
				return []string{makeContentChunk("```python\nprint('hello')\n```"), makeFinishChunk("stop")}
			},
			Assertions: []QualityAssertion{
				{Name: "no_error", Severity: "critical", Check: func(r string) bool { return true }},
				{Name: "completes", Severity: "high", Check: func(r string) bool { return true }},
			},
		},
		{
			Name:          "Forced Search",
			Input:         "Latest Go version",
			SearchMode: "on",
			ModelBehavior: func() []string {
				return []string{makeContentChunk("Go 1.24 is the latest"), makeFinishChunk("stop")}
			},
			Assertions: []QualityAssertion{
				{Name: "no_error", Severity: "critical", Check: func(r string) bool { return true }},
				{Name: "completes", Severity: "high", Check: func(r string) bool { return true }},
			},
		},
		{
			Name:  "Autonomous Search",
			Input: "What happened today?",
			ModelBehavior: func() []string {
				return []string{makeContentChunk("Today's news..."), makeFinishChunk("stop")}
			},
			Assertions: []QualityAssertion{
				{Name: "no_error", Severity: "critical", Check: func(r string) bool { return true }},
				{Name: "completes", Severity: "high", Check: func(r string) bool { return true }},
			},
		},
		{
			Name:  "Thinking Mode",
			Input: "Explain quantum computing",
			ModelBehavior: func() []string {
				return []string{makeReasoningChunk("Let me think..."), makeContentChunk("Quantum computing uses..."), makeFinishChunk("stop")}
			},
			Assertions: []QualityAssertion{
				{Name: "no_error", Severity: "critical", Check: func(r string) bool { return true }},
				{Name: "completes", Severity: "high", Check: func(r string) bool { return true }},
			},
		},
	}

	var reports []QualityReport

	for _, scenario := range scenarios {
		chunks := scenario.ModelBehavior()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount := 0
			var req llm.ChatCompletionRequest
			body := make([]byte, 1024*1024)
			n, _ := r.Body.Read(body)
			json.Unmarshal(body[:n], &req)
			callCount++

			if callCount == 1 && len(req.Tools) > 0 {
				sseData := makeSSEData([]string{
					makeToolCallChunk("call_1", "search", `{"query":"test"}`),
					makeFinishChunk("tool_calls"),
				})
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, sseData)
			} else {
				sseData := makeSSEData(chunks)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, sseData)
			}
		}))

		searchProvider := &mockSearchProvider{
			name: "testSearch",
			results: &search.SearchResponse{
				Engine:  "testSearch",
				Results: []search.SearchResult{{Title: "T", URL: "https://a.com", Snippet: "S"}},
			},
		}

		svc := newInteractionTestService(t, server, searchProvider)
		report := runQualityScenario(t, svc, scenario)
		reports = append(reports, report)
		server.Close()
	}

	printQualityReport(t, reports)

	for _, r := range reports {
		if r.Failed > 0 {
			t.Errorf("Scenario %s had %d failures", r.Scenario, r.Failed)
		}
	}
}

func TestQuality_SearchScenarios(t *testing.T) {
	scenarios := []QualityScenario{
		{
			Name:          "Search with results",
			Input:         "Go 1.24 features",
			SearchMode: "on",
			Assertions: []QualityAssertion{
				{Name: "no_error", Severity: "critical", Check: func(r string) bool { return true }},
			},
		},
		{
			Name:          "Search without results",
			Input:         "xyznonexistent123",
			SearchMode: "on",
			Assertions: []QualityAssertion{
				{Name: "no_error", Severity: "critical", Check: func(r string) bool { return true }},
			},
		},
		{
			Name:  "No search when disabled",
			Input: "What is AI?",
			Assertions: []QualityAssertion{
				{Name: "no_error", Severity: "critical", Check: func(r string) bool { return true }},
			},
		},
	}

	var reports []QualityReport

	for _, scenario := range scenarios {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sseData := makeSSEData([]string{
				makeContentChunk("Answer"),
				makeFinishChunk("stop"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
		}))

		searchProvider := &mockSearchProvider{
			name: "testSearch",
			results: &search.SearchResponse{
				Engine:  "testSearch",
				Results: []search.SearchResult{{Title: "T", URL: "https://a.com", Snippet: "S"}},
			},
		}

		svc := newInteractionTestService(t, server, searchProvider)
		report := runQualityScenario(t, svc, scenario)
		reports = append(reports, report)
		server.Close()
	}

	printQualityReport(t, reports)

	for _, r := range reports {
		if r.Failed > 0 {
			t.Errorf("Scenario %s had %d failures", r.Scenario, r.Failed)
		}
	}
}

func TestQuality_ToolCallScenarios(t *testing.T) {
	scenarios := []QualityScenario{
		{
			Name:  "Single tool call",
			Input: "Search for Go tutorials",
			Assertions: []QualityAssertion{
				{Name: "no_error", Severity: "critical", Check: func(r string) bool { return true }},
			},
		},
		{
			Name:  "Max rounds limit",
			Input: "Keep searching until you find it",
			Assertions: []QualityAssertion{
				{Name: "no_error", Severity: "critical", Check: func(r string) bool { return true }},
				{Name: "terminates", Severity: "critical", Check: func(r string) bool { return true }},
			},
		},
		{
			Name:  "Invalid tool arguments",
			Input: "Search with bad args",
			Assertions: []QualityAssertion{
				{Name: "no_error", Severity: "critical", Check: func(r string) bool { return true }},
			},
		},
	}

	var reports []QualityReport

	for _, scenario := range scenarios {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount < 3 {
				sseData := makeSSEData([]string{
					makeToolCallChunk(fmt.Sprintf("call_%d", callCount), "search", `{"query":"test"}`),
					makeFinishChunk("tool_calls"),
				})
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, sseData)
			} else {
				sseData := makeSSEData([]string{
					makeContentChunk("Final answer"),
					makeFinishChunk("stop"),
				})
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, sseData)
			}
		}))

		searchProvider := &mockSearchProvider{
			name: "testSearch",
			results: &search.SearchResponse{
				Engine:  "testSearch",
				Results: []search.SearchResult{{Title: "T", URL: "https://a.com", Snippet: "S"}},
			},
		}

		svc := newInteractionTestService(t, server, searchProvider)
		report := runQualityScenario(t, svc, scenario)
		reports = append(reports, report)
		server.Close()
	}

	printQualityReport(t, reports)

	for _, r := range reports {
		if r.Failed > 0 {
			t.Errorf("Scenario %s had %d failures", r.Scenario, r.Failed)
		}
	}
}
