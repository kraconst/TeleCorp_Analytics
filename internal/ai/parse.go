package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content *string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// parseCompletion разбирает ответ chat completions. Текст ошибок не содержит тело ответа.
func parseCompletion(body []byte) (*usecase.AIAnalysisResult, error) {
	var resp chatCompletionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%w: ai response is not a valid chat completion", domain.ErrBadGateway)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("%w: ai response has no choices", domain.ErrBadGateway)
	}
	content := resp.Choices[0].Message.Content
	if content == nil || strings.TrimSpace(*content) == "" {
		return nil, fmt.Errorf("%w: ai response content is empty", domain.ErrBadGateway)
	}
	return parseAnalysis(*content)
}

// parseAnalysis разбирает JSON-анализ модели (Q10, 2A-8, 2A-8a, 2A-9).
// Строго (ErrBadGateway): не JSON-объект, отсутствующие/null/нестроковые topic, agreements,
// next_step, open_questions, risk и недопустимый risk. Значение risk по умолчанию не подставляется.
// Терпимо: suggested_status и alert_types; отброшенные значения пишутся в лог.
func parseAnalysis(content string) (*usecase.AIAnalysisResult, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stripCodeFence(content)), &obj); err != nil || obj == nil {
		return nil, fmt.Errorf("%w: ai analysis is not a JSON object", domain.ErrBadGateway)
	}

	var res usecase.AIAnalysisResult
	fields := []struct {
		key string
		dst *string
	}{
		{"topic", &res.Topic},
		{"agreements", &res.Agreements},
		{"next_step", &res.NextStep},
		{"open_questions", &res.OpenQuestions},
	}
	for _, f := range fields {
		v, err := requiredString(obj, f.key)
		if err != nil {
			return nil, err
		}
		*f.dst = v
	}

	riskRaw, err := requiredString(obj, "risk")
	if err != nil {
		return nil, err
	}
	risk, ok := normalizeRisk(riskRaw)
	if !ok {
		logValue("rejected", "risk", textLogValue(riskRaw), "unsupported value")
		return nil, fmt.Errorf("%w: ai analysis field \"risk\" has unsupported value", domain.ErrBadGateway)
	}
	res.Risk = risk

	sRaw, sPresent := obj["suggested_status"]
	res.SuggestedStatus = parseSuggestedStatus(sRaw, sPresent)
	aRaw, aPresent := obj["alert_types"]
	res.AlertTypes = parseAlertTypes(aRaw, aPresent)
	return &res, nil
}

// stripCodeFence снимает обёртку ```json … ``` или ``` … ```.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = s[len("```"):]
	if nl := strings.IndexByte(s, '\n'); nl >= 0 {
		if tag := strings.TrimSpace(s[:nl]); tag == "" || strings.EqualFold(tag, "json") {
			s = s[nl+1:]
		}
	} else if len(s) >= len("json") && strings.EqualFold(s[:len("json")], "json") {
		s = s[len("json"):]
	}
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func requiredString(obj map[string]json.RawMessage, key string) (string, error) {
	raw, ok := obj[key]
	if !ok {
		return "", fmt.Errorf("%w: ai analysis is missing field %q", domain.ErrBadGateway, key)
	}
	if isJSONNull(raw) {
		return "", fmt.Errorf("%w: ai analysis field %q is null", domain.ErrBadGateway, key)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("%w: ai analysis field %q is not a string", domain.ErrBadGateway, key)
	}
	return strings.TrimSpace(s), nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func normalizeEnum(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func normalizeRisk(s string) (domain.RiskLevel, bool) {
	v := domain.RiskLevel(normalizeEnum(s))
	for _, r := range allowedRisks {
		if v == r {
			return r, true
		}
	}
	return "", false
}

// parseSuggestedStatus: отсутствие, null, нестроковый тип или неизвестное значение → nil.
func parseSuggestedStatus(raw json.RawMessage, present bool) *domain.ChatStatus {
	if !present || isJSONNull(raw) {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		logValue("dropped", "suggested_status", rawLogValue(raw), "not a string")
		return nil
	}
	v := domain.ChatStatus(normalizeEnum(s))
	for _, st := range allowedStatuses {
		if v == st {
			out := st
			return &out
		}
	}
	logValue("dropped", "suggested_status", textLogValue(s), "unknown value")
	return nil
}

// parseAlertTypes: отсутствие, null или значение иной формы, чем массив строк, → пустой список.
// В массиве строк отбрасываются неизвестные значения, no_reply и дубли. Порядок сохраняется.
// Результат всегда non-nil.
func parseAlertTypes(raw json.RawMessage, present bool) []domain.AlertType {
	out := []domain.AlertType{}
	if !present || isJSONNull(raw) {
		return out
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		logValue("dropped", "alert_types", rawLogValue(raw), "not an array")
		return out
	}
	values := make([]string, len(items))
	for i, item := range items {
		if isJSONNull(item) || json.Unmarshal(item, &values[i]) != nil {
			for _, dropped := range items {
				logValue("dropped", "alert_types", rawLogValue(dropped), "array contains a non-string")
			}
			return out
		}
	}
	seen := make(map[domain.AlertType]bool, len(values))
	for _, s := range values {
		v := domain.AlertType(normalizeEnum(s))
		switch {
		case v == domain.AlertTypeNoReply:
			logValue("dropped", "alert_types", string(domain.AlertTypeNoReply), "no_reply is set by the system only")
		case !isAllowedAlertType(v):
			logValue("dropped", "alert_types", textLogValue(s), "unknown value")
		case seen[v]:
			logValue("dropped", "alert_types", string(v), "duplicate")
		default:
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func isAllowedAlertType(v domain.AlertType) bool {
	for _, a := range allowedAlertTypes {
		if v == a {
			return true
		}
	}
	return false
}

// logValue принимает только безопасную метку или значение из проверенного перечисления.
func logValue(action, field, safeValue, reason string) {
	log.Printf("ai: %s %s value %q: %s", action, field, safeValue, reason)
}

// textLogValue классифицирует исходный текст по Q8, не сохраняя его фрагментов.
func textLogValue(value string) string {
	if maskPhones(value) != value {
		return phoneMask
	}
	return "[текст]"
}

// rawLogValue проверяет строки и числа внутри JSON по Q8. Декодер восстанавливает переносы
// и Unicode из escape-последовательностей. Содержимое ключей и значений остаётся вне лога.
func rawLogValue(raw json.RawMessage) string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	for {
		token, err := dec.Token()
		if err != nil {
			// Исходный JSON уже провалидирован перед разбором полей.
			return "[JSON]"
		}
		if number, ok := token.(json.Number); ok {
			token = number.String()
		}
		if value, ok := token.(string); ok && textLogValue(value) == phoneMask {
			return phoneMask
		}
	}
}
