package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

// analysisJSON возвращает валидный JSON-анализ модели; mutate может удалить или подменить ключи.
// Значения типа json.RawMessage вставляются как есть.
func analysisJSON(t *testing.T, mutate func(m map[string]any)) string {
	t.Helper()
	m := map[string]any{
		"topic":            "Поставка труб",
		"agreements":       "Скидка 5% при оплате до пятницы",
		"next_step":        "Выслать КП",
		"open_questions":   "Сроки доставки",
		"risk":             "medium",
		"suggested_status": "negotiation",
		"alert_types":      []any{"urgent"},
	}
	if mutate != nil {
		mutate(m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal analysis: %v", err)
	}
	return string(b)
}

// analyzeContent прогоняет content (ответ модели) через httptest.Server и AnalyzeChat.
func analyzeContent(t *testing.T, content string) (*usecase.AIAnalysisResult, error) {
	t.Helper()
	return analyzeBody(t, envelope(t, content))
}

// analyzeBody отдаёт body как тело ответа 200 и вызывает AnalyzeChat.
func analyzeBody(t *testing.T, body string) (*usecase.AIAnalysisResult, error) {
	t.Helper()
	s := newStubServer(t, respondWith(http.StatusOK, body, nil))
	c := newTestClient(t, s.srv.URL, testTimeout, testMaxChars)
	return c.AnalyzeChat(context.Background(), testInput())
}

func statusPtr(s domain.ChatStatus) *domain.ChatStatus { return &s }

func TestAnalyzeChat_ValidResponse(t *testing.T) {
	res, err := analyzeContent(t, analysisJSON(t, nil))
	if err != nil {
		t.Fatalf("AnalyzeChat: %v", err)
	}
	want := &usecase.AIAnalysisResult{
		Topic:           "Поставка труб",
		Agreements:      "Скидка 5% при оплате до пятницы",
		NextStep:        "Выслать КП",
		OpenQuestions:   "Сроки доставки",
		Risk:            domain.RiskMedium,
		SuggestedStatus: statusPtr(domain.ChatStatusNegotiation),
		AlertTypes:      []domain.AlertType{domain.AlertTypeUrgent},
	}
	if !reflect.DeepEqual(res, want) {
		t.Errorf("result = %+v, want %+v", res, want)
	}
}

// 2A-9: обёртка в Markdown code fence снимается.
func TestAnalyzeChat_CodeFencedJSON(t *testing.T) {
	js := analysisJSON(t, nil)
	tests := map[string]string{
		"json fence":                  "```json\n" + js + "\n```",
		"json fence with spaces":      "  \n```json\n" + js + "\n```  \n",
		"uppercase JSON tag":          "```JSON\n" + js + "\n```",
		"plain fence":                 "```\n" + js + "\n```",
		"single line json fence":      "```json" + js + "```",
		"fence without closing ticks": "```json\n" + js,
		"no fence":                    js,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			res, err := analyzeContent(t, content)
			if err != nil {
				t.Fatalf("AnalyzeChat: %v", err)
			}
			if res.Topic != "Поставка труб" || res.Risk != domain.RiskMedium {
				t.Errorf("unexpected result: %+v", res)
			}
		})
	}
}

// 2A-8: невалидный ответ провайдера → ErrBadGateway, результат не возвращается.
func TestAnalyzeChat_MalformedResponseIsBadGateway(t *testing.T) {
	js := analysisJSON(t, nil)
	tests := []struct {
		name string
		body string
	}{
		{"envelope not JSON", "not json at all"},
		{"envelope truncated", `{"choices":[{"message":{"content":"{}"`},
		{"envelope is array", `[1,2,3]`},
		{"no choices key", `{}`},
		{"empty choices", `{"choices":[]}`},
		{"null content", `{"choices":[{"message":{"content":null}}]}`},
		{"missing content", `{"choices":[{"message":{"role":"assistant"}}]}`},
		{"blank content", envelope(t, "  \n ")},
		{"content not JSON", envelope(t, "Тема: поставка труб, риск высокий")},
		{"content truncated JSON", envelope(t, js[:len(js)/2])},
		{"content JSON array", envelope(t, `[`+js+`]`)},
		{"content JSON string", envelope(t, `"`+strings.ReplaceAll(js, `"`, `\"`)+`"`)},
		{"content JSON null", envelope(t, "null")},
		{"content JSON number", envelope(t, "42")},
		{"fenced broken JSON", envelope(t, "```json\n{\"topic\": \n```")},
		{"text around JSON", envelope(t, "Вот результат: "+js)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := analyzeBody(t, tt.body)
			if res != nil {
				t.Errorf("result must be nil on error, got %+v", res)
			}
			assertErrClass(t, err, errClass{badGateway: true})
		})
	}
}

// 2A-8: отсутствующий, null или нестроковый обязательный ключ → ErrBadGateway.
func TestAnalyzeChat_RequiredFieldsStrict(t *testing.T) {
	keys := []string{"topic", "agreements", "next_step", "open_questions", "risk"}
	invalid := []struct {
		name  string
		value any // nil — ключ удаляется
	}{
		{"missing", nil},
		{"null", json.RawMessage(`null`)},
		{"number", json.RawMessage(`42`)},
		{"bool", json.RawMessage(`true`)},
		{"object", json.RawMessage(`{"text":"high"}`)},
		{"array", json.RawMessage(`["high"]`)},
	}
	for _, key := range keys {
		for _, inv := range invalid {
			t.Run(key+"/"+inv.name, func(t *testing.T) {
				content := analysisJSON(t, func(m map[string]any) {
					if inv.value == nil {
						delete(m, key)
						return
					}
					m[key] = inv.value
				})
				res, err := analyzeContent(t, content)
				if res != nil {
					t.Errorf("result must be nil on error, got %+v", res)
				}
				assertErrClass(t, err, errClass{badGateway: true})
			})
		}
	}
}

// 2A-8: пустые строки в текстовых полях допустимы.
func TestAnalyzeChat_EmptyTextFieldsAllowed(t *testing.T) {
	content := analysisJSON(t, func(m map[string]any) {
		m["topic"] = ""
		m["agreements"] = ""
		m["next_step"] = "   "
		m["open_questions"] = ""
	})
	res, err := analyzeContent(t, content)
	if err != nil {
		t.Fatalf("AnalyzeChat: %v", err)
	}
	if res.Topic != "" || res.Agreements != "" || res.NextStep != "" || res.OpenQuestions != "" {
		t.Errorf("text fields = %q, %q, %q, %q; want empty", res.Topic, res.Agreements, res.NextStep, res.OpenQuestions)
	}
	if res.Risk != domain.RiskMedium {
		t.Errorf("risk = %q, want medium", res.Risk)
	}
}

// 2A-8a: risk нормализуется trim + lower.
func TestAnalyzeChat_RiskNormalized(t *testing.T) {
	tests := []struct {
		raw  string
		want domain.RiskLevel
	}{
		{"High", domain.RiskHigh},
		{" HIGH ", domain.RiskHigh},
		{"high", domain.RiskHigh},
		{"Medium\n", domain.RiskMedium},
		{"\tLOW", domain.RiskLow},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			res, err := analyzeContent(t, analysisJSON(t, func(m map[string]any) { m["risk"] = tt.raw }))
			if err != nil {
				t.Fatalf("AnalyzeChat: %v", err)
			}
			if res.Risk != tt.want {
				t.Errorf("risk = %q, want %q", res.Risk, tt.want)
			}
		})
	}
}

// 2A-8: недопустимый после нормализации risk → ErrBadGateway, значение по умолчанию не подставляется.
func TestAnalyzeChat_InvalidRiskIsBadGateway(t *testing.T) {
	for _, raw := range []string{"", "   ", "critical", "высокий", "hi gh", "low-medium", "none"} {
		t.Run(raw, func(t *testing.T) {
			captureLog(t)
			res, err := analyzeContent(t, analysisJSON(t, func(m map[string]any) { m["risk"] = raw }))
			if res != nil {
				t.Errorf("result must be nil on error, got %+v", res)
			}
			assertErrClass(t, err, errClass{badGateway: true})
		})
	}
}

// 2A-8a, Q10: suggested_status разбирается терпимо.
func TestAnalyzeChat_SuggestedStatus(t *testing.T) {
	tests := []struct {
		name  string
		value any // nil — ключ удаляется
		want  *domain.ChatStatus
	}{
		{"valid", "lost", statusPtr(domain.ChatStatusLost)},
		{"mixed case with spaces", " Waiting_Reply ", statusPtr(domain.ChatStatusWaitingReply)},
		{"upper case", "ACTION_REQUIRED", statusPtr(domain.ChatStatusActionRequired)},
		{"unknown value", "in_progress", nil},
		{"translated value", "переговоры", nil},
		{"empty string", "", nil},
		{"number", json.RawMessage(`3`), nil},
		{"bool", json.RawMessage(`false`), nil},
		{"object", json.RawMessage(`{"status":"lost"}`), nil},
		{"array", json.RawMessage(`["lost"]`), nil},
		{"null", json.RawMessage(`null`), nil},
		{"missing", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captureLog(t)
			res, err := analyzeContent(t, analysisJSON(t, func(m map[string]any) {
				if tt.value == nil {
					delete(m, "suggested_status")
					return
				}
				m["suggested_status"] = tt.value
			}))
			if err != nil {
				t.Fatalf("AnalyzeChat: %v (optional field must not fail the analysis)", err)
			}
			if !reflect.DeepEqual(res.SuggestedStatus, tt.want) {
				t.Errorf("suggested_status = %v, want %v", deref(res.SuggestedStatus), deref(tt.want))
			}
			if res.Topic != "Поставка труб" || res.Risk != domain.RiskMedium {
				t.Errorf("rest of result changed: %+v", res)
			}
		})
	}
}

func deref(s *domain.ChatStatus) string {
	if s == nil {
		return "<nil>"
	}
	return string(*s)
}

// 2A-8a, Q10: alert_types — только complaint|refusal|negative|urgent, без дублей, non-nil.
func TestAnalyzeChat_AlertTypes(t *testing.T) {
	tests := []struct {
		name  string
		value any // nil — ключ удаляется
		want  []domain.AlertType
	}{
		{
			"all allowed in order",
			[]any{"complaint", "refusal", "negative", "urgent"},
			[]domain.AlertType{domain.AlertTypeComplaint, domain.AlertTypeRefusal, domain.AlertTypeNegative, domain.AlertTypeUrgent},
		},
		{
			"unknown strings filtered, case normalized, duplicates collapsed",
			json.RawMessage(`[" Complaint ","URGENT","complaint","no_reply","NO_REPLY","angry","жалоба","Refusal","urgent","negative"]`),
			[]domain.AlertType{domain.AlertTypeComplaint, domain.AlertTypeUrgent, domain.AlertTypeRefusal, domain.AlertTypeNegative},
		},
		{"only no_reply", []any{"no_reply"}, []domain.AlertType{}},
		{"only unknown", []any{"angry", "spam"}, []domain.AlertType{}},
		{"empty array", []any{}, []domain.AlertType{}},
		{"string instead of array", "complaint", []domain.AlertType{}},
		{"object instead of array", json.RawMessage(`{"complaint":true}`), []domain.AlertType{}},
		{"number instead of array", json.RawMessage(`1`), []domain.AlertType{}},
		{"null", json.RawMessage(`null`), []domain.AlertType{}},
		{"missing", nil, []domain.AlertType{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captureLog(t)
			res, err := analyzeContent(t, analysisJSON(t, func(m map[string]any) {
				if tt.value == nil {
					delete(m, "alert_types")
					return
				}
				m["alert_types"] = tt.value
			}))
			if err != nil {
				t.Fatalf("AnalyzeChat: %v (optional field must not fail the analysis)", err)
			}
			if res.AlertTypes == nil {
				t.Fatal("alert_types must be non-nil")
			}
			if !reflect.DeepEqual(res.AlertTypes, tt.want) {
				t.Errorf("alert_types = %v, want %v", res.AlertTypes, tt.want)
			}
			if res.Topic != "Поставка труб" || res.Risk != domain.RiskMedium {
				t.Errorf("rest of result changed: %+v", res)
			}
		})
	}
}

func TestAnalyzeChat_RejectsMixedAlertTypes(t *testing.T) {
	tests := []struct {
		name  string
		value json.RawMessage
	}{
		{"number", json.RawMessage(`["urgent",42]`)},
		{"null", json.RawMessage(`["urgent",null]`)},
		{"object", json.RawMessage(`["urgent",{"private-key":"private-value"}]`)},
		{"array", json.RawMessage(`["urgent",["private-value"]]`)},
		{"boolean", json.RawMessage(`["urgent",true]`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLog(t)
			res, err := analyzeContent(t, analysisJSON(t, func(m map[string]any) {
				m["alert_types"] = tt.value
			}))
			if err != nil {
				t.Fatalf("AnalyzeChat: %v", err)
			}
			if res.AlertTypes == nil || len(res.AlertTypes) != 0 {
				t.Errorf("alert_types = %#v, want non-nil empty slice", res.AlertTypes)
			}
			if res.Topic != "Поставка труб" || res.Risk != domain.RiskMedium || deref(res.SuggestedStatus) != "negotiation" {
				t.Errorf("rest of result changed: %+v", res)
			}
			const event = "ai: dropped alert_types value \"[JSON]\": array contains a non-string\n"
			if got := logs.String(); got != strings.Repeat(event, 2) {
				t.Errorf("log = %q, want one safe diagnostic per discarded element", got)
			}
		})
	}
}

// 2A-8a: каждое отброшенное значение пишется в лог (поле и значение), без текста переписки.
func TestAnalyzeChat_DroppedValuesAreLogged(t *testing.T) {
	logs := captureLog(t)
	res, err := analyzeContent(t, analysisJSON(t, func(m map[string]any) {
		m["suggested_status"] = "in_progress"
		m["alert_types"] = []any{"angry", "no_reply", "urgent", "urgent"}
	}))
	if err != nil {
		t.Fatalf("AnalyzeChat: %v", err)
	}
	if res.SuggestedStatus != nil {
		t.Errorf("suggested_status = %v, want nil", deref(res.SuggestedStatus))
	}
	if !reflect.DeepEqual(res.AlertTypes, []domain.AlertType{domain.AlertTypeUrgent}) {
		t.Errorf("alert_types = %v, want [urgent]", res.AlertTypes)
	}
	out := logs.String()
	for _, want := range []string{
		`ai: dropped suggested_status value "[текст]": unknown value`,
		`ai: dropped alert_types value "[текст]": unknown value`,
		`ai: dropped alert_types value "no_reply": no_reply is set by the system only`,
		`ai: dropped alert_types value "urgent": duplicate`,
	} {
		if got := strings.Count(out, want); got != 1 {
			t.Errorf("log contains %q %d times, want 1; log:\n%s", want, got, out)
		}
	}
	for _, leaked := range []string{"in_progress", "angry", testAPIKey} {
		if strings.Contains(out, leaked) {
			t.Error("log contains an unsafe value")
		}
	}
}

func TestAnalyzeChat_DroppedValuesDoNotLeakConversation(t *testing.T) {
	logs := captureLog(t)
	msgs := testInput().Messages
	res, err := analyzeContent(t, analysisJSON(t, func(m map[string]any) {
		m["suggested_status"] = msgs[0].Text
		m["alert_types"] = map[string]any{
			"payload": map[string]string{msgs[1].Text: msgs[0].Text},
		}
	}))
	if err != nil {
		t.Fatalf("AnalyzeChat: %v", err)
	}
	want := &usecase.AIAnalysisResult{
		Topic:         "Поставка труб",
		Agreements:    "Скидка 5% при оплате до пятницы",
		NextStep:      "Выслать КП",
		OpenQuestions: "Сроки доставки",
		Risk:          domain.RiskMedium,
		AlertTypes:    []domain.AlertType{},
	}
	if !reflect.DeepEqual(res, want) {
		t.Errorf("AnalyzeChat = %+v, want %+v", res, want)
	}

	out := logs.String()
	for _, event := range []string{
		`ai: dropped suggested_status value "[текст]": unknown value`,
		`ai: dropped alert_types value "[JSON]": not an array`,
	} {
		if got := strings.Count(out, event); got != 1 {
			t.Errorf("log contains %q %d times, want 1", event, got)
		}
	}
	for _, leaked := range []string{msgs[0].Text, msgs[1].Text, "payload", "map[", testAPIKey} {
		if strings.Contains(out, leaked) {
			t.Error("log contains model text or serialized JSON")
		}
	}
}

// 2A-8a, Q8: значение в логе маскируется, включая номер, разорванный "\n"/"\r\n",
// и Unicode-варианты (тире, пробелы, полноширинные цифры, невидимые символы).
func TestLogValue_MasksPhones(t *testing.T) {
	tests := []struct {
		name, value string
	}{
		{"LF", "8 999\n123 45 67"},
		{"CRLF", "8 999\r\n123 45 67"},
		{"CRLF with parens", "+7 (999)\r\n123-45-67"},
		{"CR", "8999\r1234567"},
		{"unicode dashes and minus", "8\u2011999\u2013123\u201445\u221267"},
		{"unicode spaces", "8\u00A0999\u202F123\u200945 67"},
		{"fullwidth", "＋７（９９９）１２３４５６７"},
		{"invisible", "8916\u200B123\u00AD45\u206067"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLog(t)
			logValue("dropped", "suggested_status", textLogValue(tt.value), "unknown value")
			out := logs.String()
			if want := `ai: dropped suggested_status value "[телефон]": unknown value`; !strings.Contains(out, want) {
				t.Errorf("log = %q, want it to contain %q", out, want)
			}
			for _, leaked := range []string{"999", "９９９", "123", "１２３"} {
				if strings.Contains(out, leaked) {
					t.Errorf("phone fragment %q leaked into log: %q", leaked, out)
				}
			}
		})
	}
}

// jsonEscapeDigits записывает каждую ASCII-цифру s как JSON-escape (8 → \u0038), прочие символы — как есть.
func jsonEscapeDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteString("\\u003")
		}
		b.WriteRune(r)
	}
	return b.String()
}

// 2A-8a, Q8: отброшенные значения из ответа модели маскируются в логе, в том числе внутри
// нестроковых JSON-значений, где перевод строки экранирован ("\n" в сыром JSON), а цифры
// записаны JSON-escape (\u0038).
func TestAnalyzeChat_DroppedValuesLogMasksPhones(t *testing.T) {
	logs := captureLog(t)
	res, err := analyzeContent(t, analysisJSON(t, func(m map[string]any) {
		m["suggested_status"] = "8 999\r\n123 45 67"
		m["alert_types"] = json.RawMessage(`["8 999\n765 43 21", {"phone":"8 999\n555 66 77"}, 89161234567, "` +
			jsonEscapeDigits("89162223344") + `", {"tel":"` + jsonEscapeDigits(`8 916\n333 44 55`) + `"}, "urgent"]`)
	}))
	if err != nil {
		t.Fatalf("AnalyzeChat: %v", err)
	}
	if res.SuggestedStatus != nil {
		t.Errorf("suggested_status = %v, want nil", deref(res.SuggestedStatus))
	}
	if res.AlertTypes == nil || len(res.AlertTypes) != 0 {
		t.Errorf("alert_types = %#v, want non-nil empty slice", res.AlertTypes)
	}
	out := logs.String()
	if n := strings.Count(out, "[телефон]"); n != 6 {
		t.Errorf("log must contain 6 masked values, got %d; log:\n%s", n, out)
	}
	for _, leaked := range []string{"999", "123 45 67", "765 43 21", "555 66 77", "8916", "2223344", "333 44 55", "e+10", "u003"} {
		if strings.Contains(out, leaked) {
			t.Errorf("phone fragment %q leaked into log; log:\n%s", leaked, out)
		}
	}
}

// Ответ провайдера больше 1 MiB отклоняется, даже если он валиден.
func TestAnalyzeChat_OversizedResponseIsBadGateway(t *testing.T) {
	content := analysisJSON(t, func(m map[string]any) {
		m["topic"] = strings.Repeat("я", maxResponseBytes/2+1) // > 1 MiB в UTF-8
	})
	res, err := analyzeContent(t, content)
	if res != nil {
		t.Error("result must be nil on error")
	}
	assertErrClass(t, err, errClass{badGateway: true})
}
