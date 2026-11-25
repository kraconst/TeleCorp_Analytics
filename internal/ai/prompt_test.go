package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

// sentRequest — тело запроса к провайдеру, декодированное независимо от типов пакета.
type sentRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Temperature *float64 `json:"temperature"`
}

func decodeSentRequest(t *testing.T, req capturedRequest) sentRequest {
	t.Helper()
	var body sentRequest
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("request body is not valid JSON: %v", err)
	}
	return body
}

// sendAndCaptureUserContent вызывает AnalyzeChat с лимитом maxChars и возвращает user-сообщение,
// перехваченное httptest.Server (2A-10).
func sendAndCaptureUserContent(t *testing.T, maxChars int, in usecase.AnalyzeChatInput) string {
	t.Helper()
	s := newStubServer(t, respondWith(http.StatusOK, envelope(t, analysisJSON(t, nil)), nil))
	c := newTestClient(t, s.srv.URL, testTimeout, maxChars)
	if _, err := c.AnalyzeChat(context.Background(), in); err != nil {
		t.Fatalf("AnalyzeChat: %v", err)
	}
	body := decodeSentRequest(t, s.lastRequest(t))
	if len(body.Messages) != 2 || body.Messages[1].Role != "user" {
		t.Fatalf("unexpected request messages: %+v", body.Messages)
	}
	return body.Messages[1].Content
}

var baseTime = time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)

func msg(id int64, dir domain.MessageDirection, text string, at time.Time) domain.Message {
	return domain.Message{ID: id, ChatID: 7, Direction: dir, Text: text, SentAt: at}
}

func assertInOrder(t *testing.T, content string, parts ...string) {
	t.Helper()
	pos := -1
	for _, p := range parts {
		i := strings.Index(content, p)
		if i < 0 {
			t.Fatalf("content does not contain %q; content:\n%s", p, content)
		}
		if i <= pos {
			t.Fatalf("%q is out of order; content:\n%s", p, content)
		}
		pos = i
	}
}

// 2A-10: хронологический порядок, роли «Клиент»/«Менеджер», время в UTC.
func TestAnalyzeChat_ContextChronologyRolesAndTime(t *testing.T) {
	msk := time.FixedZone("MSK", 3*60*60)
	photo := domain.MediaTypePhoto
	in := usecase.AnalyzeChatInput{Messages: []domain.Message{
		msg(1, domain.MessageDirectionIn, "Здравствуйте, нужны трубы", baseTime),
		msg(2, domain.MessageDirectionOut, "Добрый день, пришлю КП", time.Date(2026, 3, 10, 12, 5, 0, 0, msk)),
		{ID: 3, ChatID: 7, Direction: domain.MessageDirectionIn, Text: "Вот схема\nвторая строка", MediaType: &photo, SentAt: baseTime.Add(10 * time.Minute)},
		msg(4, domain.MessageDirectionOut, "Спасибо", baseTime.Add(24*time.Hour+time.Minute)),
	}}
	content := sendAndCaptureUserContent(t, testMaxChars, in)

	assertInOrder(t, content,
		"Переписка (время UTC, от старых сообщений к новым):\n",
		"[2026-03-10 09:00] Клиент: Здравствуйте, нужны трубы\n",
		"[2026-03-10 09:05] Менеджер: Добрый день, пришлю КП\n",
		"[2026-03-10 09:10] Клиент: Вот схема вторая строка [вложение: photo]\n",
		"[2026-03-11 09:01] Менеджер: Спасибо\n",
	)
	if strings.Contains(content, "Предыдущая выжимка") {
		t.Error("previous summary block must be absent when Previous is nil")
	}
	if strings.Contains(content, "опущены") || strings.Contains(content, "обрезан") {
		t.Errorf("no messages must be dropped or truncated; content:\n%s", content)
	}
}

func TestAnalyzeChat_ContextUsesFixedAttachmentLabels(t *testing.T) {
	photo, document, voice := domain.MediaTypePhoto, domain.MediaTypeDocument, domain.MediaTypeVoice
	unknown := domain.MediaType("+1 (202) 555-\n0143")
	in := usecase.AnalyzeChatInput{Messages: []domain.Message{
		{ID: 1, Direction: domain.MessageDirectionIn, Text: "Фото", MediaType: &photo, SentAt: baseTime},
		{ID: 2, Direction: domain.MessageDirectionIn, Text: "Смета", MediaType: &document, SentAt: baseTime.Add(time.Minute)},
		{ID: 3, Direction: domain.MessageDirectionOut, Text: "Ответ", MediaType: &voice, SentAt: baseTime.Add(2 * time.Minute)},
		{ID: 4, Direction: domain.MessageDirectionIn, Text: "Файл", MediaType: &unknown, SentAt: baseTime.Add(3 * time.Minute)},
		msg(5, domain.MessageDirectionOut, "Спасибо", baseTime.Add(4*time.Minute)),
	}}
	content := sendAndCaptureUserContent(t, testMaxChars, in)

	for _, fragment := range []string{"+1 (202) 555", "0143"} {
		if strings.Contains(content, fragment) {
			t.Errorf("attachment metadata exposed phone fragment %q", fragment)
		}
	}
	assertInOrder(t, content,
		"Клиент: Фото [вложение: photo]\n",
		"Клиент: Смета [вложение: document]\n",
		"Менеджер: Ответ [вложение: voice]\n",
		"Клиент: Файл [вложение]\n",
		"Менеджер: Спасибо\n",
	)
}

// 2A-10: старейшие сообщения отбрасываются, пока сумма рун > maxChars.
func TestAnalyzeChat_ContextDropsOldestByRunes(t *testing.T) {
	in := usecase.AnalyzeChatInput{Messages: []domain.Message{
		msg(1, domain.MessageDirectionIn, "абвгдежзий", baseTime), // 10 рун, 20 байт
		msg(2, domain.MessageDirectionOut, "клмно", baseTime.Add(time.Minute)),
		msg(3, domain.MessageDirectionIn, "прсту", baseTime.Add(2*time.Minute)),
	}}
	// Сумма 20 рун > 10: отбрасывается только первое; оставшиеся 10 рун (20 байт) укладываются в лимит,
	// что выполняется только при подсчёте в рунах.
	content := sendAndCaptureUserContent(t, 10, in)

	if strings.Contains(content, "абвгдежзий") {
		t.Errorf("oldest message must be dropped; content:\n%s", content)
	}
	assertInOrder(t, content,
		"(более ранние сообщения опущены: 1)\n",
		"[2026-03-10 09:01] Менеджер: клмно\n",
		"[2026-03-10 09:02] Клиент: прсту\n",
	)
	if strings.Contains(content, "обрезан") {
		t.Errorf("no message must be truncated; content:\n%s", content)
	}
}

// 2A-10: сумма, равная лимиту, не приводит к отбрасыванию.
func TestAnalyzeChat_ContextKeepsAllAtExactLimit(t *testing.T) {
	in := usecase.AnalyzeChatInput{Messages: []domain.Message{
		msg(1, domain.MessageDirectionIn, "абвгд", baseTime),
		msg(2, domain.MessageDirectionOut, "еёжзи", baseTime.Add(time.Minute)),
	}}
	content := sendAndCaptureUserContent(t, 10, in)

	assertInOrder(t, content, "Клиент: абвгд\n", "Менеджер: еёжзи\n")
	if strings.Contains(content, "опущены") || strings.Contains(content, "обрезан") {
		t.Errorf("nothing must be dropped or truncated; content:\n%s", content)
	}
}

// 2A-10: одиночное сообщение длиннее лимита обрезается по рунам.
func TestAnalyzeChat_ContextTruncatesSingleLongMessage(t *testing.T) {
	t.Run("only message", func(t *testing.T) {
		in := usecase.AnalyzeChatInput{Messages: []domain.Message{
			msg(1, domain.MessageDirectionIn, "абвгдежзий", baseTime),
		}}
		content := sendAndCaptureUserContent(t, 5, in)

		if !strings.Contains(content, "[2026-03-10 09:00] Клиент: абвгд\n") {
			t.Errorf("message must be truncated to 5 runes; content:\n%s", content)
		}
		if strings.Contains(content, "абвгде") {
			t.Errorf("message is longer than 5 runes; content:\n%s", content)
		}
		if !strings.Contains(content, "(текст сообщения обрезан по лимиту длины)\n") {
			t.Errorf("truncation note is missing; content:\n%s", content)
		}
		if strings.Contains(content, "опущены") {
			t.Errorf("nothing must be dropped; content:\n%s", content)
		}
	})

	t.Run("newest remaining message", func(t *testing.T) {
		in := usecase.AnalyzeChatInput{Messages: []domain.Message{
			msg(1, domain.MessageDirectionOut, "старое", baseTime),
			msg(2, domain.MessageDirectionIn, "ёжиквтумане", baseTime.Add(time.Minute)),
		}}
		content := sendAndCaptureUserContent(t, 4, in)

		if strings.Contains(content, "старое") {
			t.Errorf("oldest message must be dropped; content:\n%s", content)
		}
		assertInOrder(t, content,
			"(более ранние сообщения опущены: 1)\n",
			"(текст сообщения обрезан по лимиту длины)\n",
			"[2026-03-10 09:01] Клиент: ёжик\n",
		)
		if strings.Contains(content, "ёжикв") {
			t.Errorf("message is longer than 4 runes; content:\n%s", content)
		}
	})
}

// 2A-10, Q8: телефоны (10–15 цифр) и цепочки из нескольких номеров подряд (16+ цифр)
// заменяются на [телефон], короткие числа не трогаются.
func TestAnalyzeChat_ContextMasksPhones(t *testing.T) {
	in := usecase.AnalyzeChatInput{Messages: []domain.Message{
		msg(1, domain.MessageDirectionIn, "Звоните +7 (999) 123-45-67 или 89991234567, заказ 42, код 12345", baseTime),
		msg(2, domain.MessageDirectionOut, "Мой рабочий 8 999 765 43 21", baseTime.Add(time.Minute)),
		msg(3, domain.MessageDirectionIn, "Два номера: 89991234567 89997654321, ИНН 7701234567 89991234567", baseTime.Add(2*time.Minute)),
	}}
	content := sendAndCaptureUserContent(t, testMaxChars, in)

	assertInOrder(t, content,
		"Клиент: Звоните [телефон] или [телефон], заказ 42, код 12345\n",
		"Менеджер: Мой рабочий [телефон]\n",
		"Клиент: Два номера: [телефон], ИНН [телефон]\n",
	)
	for _, leaked := range []string{"999", "123-45-67", "89991234567", "765 43 21", "89997654321", "7701234567"} {
		if strings.Contains(content, leaked) {
			t.Errorf("phone fragment %q leaked into request; content:\n%s", leaked, content)
		}
	}
}

// 2A-10: маскирование выполняется до подсчёта длины.
func TestAnalyzeChat_ContextMasksBeforeLimit(t *testing.T) {
	in := usecase.AnalyzeChatInput{Messages: []domain.Message{
		msg(1, domain.MessageDirectionIn, "+7 (999) 123-45-67", baseTime), // 18 рун до маскирования, 9 после
	}}
	content := sendAndCaptureUserContent(t, len([]rune(phoneMask)), in)

	if !strings.Contains(content, "Клиент: [телефон]\n") {
		t.Errorf("masked phone must fit the limit intact; content:\n%s", content)
	}
	if strings.Contains(content, "обрезан") || strings.Contains(content, "999") {
		t.Errorf("message must be masked before the length limit is applied; content:\n%s", content)
	}
}

// 2A-10: Previous передаётся как предыдущая выжимка перед перепиской; телефоны в ней маскируются.
func TestAnalyzeChat_ContextIncludesPreviousSummary(t *testing.T) {
	in := testInput()
	in.Previous = &domain.ChatSummary{
		ChatID:        7,
		Topic:         "Поставка труб",
		Agreements:    "Скидка 5%",
		NextStep:      "Выслать КП",
		OpenQuestions: "Перезвонить на 89991234567",
		Risk:          domain.RiskHigh,
		LastMessageID: 1,
	}
	content := sendAndCaptureUserContent(t, testMaxChars, in)

	assertInOrder(t, content,
		"Предыдущая выжимка",
		"- topic: Поставка труб\n",
		"- agreements: Скидка 5%\n",
		"- next_step: Выслать КП\n",
		"- open_questions: Перезвонить на [телефон]\n",
		"- risk: high\n",
		"Переписка (время UTC",
		"Клиент: Здравствуйте, нужны трубы\n",
	)
	if strings.Contains(content, "89991234567") {
		t.Errorf("phone in previous summary must be masked; content:\n%s", content)
	}
}

// assertNoPhoneLeftInContent применяет assertNoPhoneLeft к данным каждой строки user-контента.
// Префикс до первого ": " (время, роль или имя поля) отбрасывается: метка времени
// "2026-03-10 09:00" после удаления разделителей Q8 сама даёт 10 цифр подряд.
func assertNoPhoneLeftInContent(t *testing.T, content string) {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		if _, data, ok := strings.Cut(line, ": "); ok {
			line = data
		}
		assertNoPhoneLeft(t, line)
	}
}

// 2A-10, Q8, R2-1: номера с LF, CRLF и CR маскируются до преобразования текста в одну строку.
func TestAnalyzeChat_ContextMasksPhonesSplitByLineBreaks(t *testing.T) {
	in := usecase.AnalyzeChatInput{Messages: []domain.Message{
		msg(1, domain.MessageDirectionIn, "Звоните 8 999\n123 45 67 вечером", baseTime),
		msg(2, domain.MessageDirectionOut, "Мой +7 (999)\r\n765-43-21", baseTime.Add(time.Minute)),
		msg(3, domain.MessageDirectionIn, "Запасной 8999\r1112233", baseTime.Add(2*time.Minute)),
		msg(4, domain.MessageDirectionOut, "Заказ 42\nна 3 шт", baseTime.Add(3*time.Minute)),
	}}
	in.Previous = &domain.ChatSummary{
		ChatID:        7,
		Topic:         "Поставка, тел. 8 999\r\n555 66 77",
		NextStep:      "Перезвонить на 8 999\n123 45 67",
		Risk:          domain.RiskLow,
		LastMessageID: 1,
	}
	content := sendAndCaptureUserContent(t, testMaxChars, in)

	assertInOrder(t, content,
		"- topic: Поставка, тел. [телефон]\n",
		"- next_step: Перезвонить на [телефон]\n",
		"Клиент: Звоните [телефон] вечером\n",
		"Менеджер: Мой [телефон]\n",
		"Клиент: Запасной [телефон]\n",
		"Менеджер: Заказ 42 на 3 шт\n",
	)
	for _, leaked := range []string{"999", "123 45 67", "765-43-21", "1112233", "555 66 77"} {
		if strings.Contains(content, leaked) {
			t.Errorf("phone fragment %q leaked into request; content:\n%s", leaked, content)
		}
	}
	assertNoPhoneLeftInContent(t, content)
}

func TestAnalyzeChat_ContextMasksBeforeFlattening(t *testing.T) {
	const splitGroups = "12 345\n678 901 руб"
	const splitMarker = "12 345 678 901\nруб"
	const date = "(2026-03-10)\n10:30"
	const amount = "12\u202f345\u00a0678\u2009901\u2007руб"
	const unicodePhone = "8\u202f999\u2028123\u200945\u200b67"
	in := usecase.AnalyzeChatInput{
		Messages: []domain.Message{
			msg(1, domain.MessageDirectionIn, "Сообщение вне лимита", baseTime),
			msg(2, domain.MessageDirectionIn, "Первый: "+splitGroups, baseTime.Add(time.Minute)),
			msg(3, domain.MessageDirectionOut, "Второй: "+splitMarker, baseTime.Add(2*time.Minute)),
			msg(4, domain.MessageDirectionIn, "Встреча "+date+", бюджет "+amount+", контакт "+unicodePhone, baseTime.Add(3*time.Minute)),
		},
		Previous: &domain.ChatSummary{
			Topic:         splitGroups,
			Agreements:    splitMarker,
			NextStep:      date,
			OpenQuestions: amount,
			Risk:          domain.RiskLevel(unicodePhone),
		},
	}
	const first = "Первый: [телефон] руб"
	const second = "Второй: [телефон] руб"
	const third = "Встреча (2026-03-10) 10:30, бюджет " + amount + ", контакт [телефон]"
	content := sendAndCaptureUserContent(t, len([]rune(first+second+third)), in)

	assertInOrder(t, content,
		"- topic: [телефон] руб\n",
		"- agreements: [телефон] руб\n",
		"- next_step: (2026-03-10) 10:30\n",
		"- open_questions: "+amount+"\n",
		"- risk: [телефон]\n",
		"(более ранние сообщения опущены: 1)\n",
		"[2026-03-10 09:01] Клиент: "+first+"\n",
		"[2026-03-10 09:02] Менеджер: "+second+"\n",
		"[2026-03-10 09:03] Клиент: "+third+"\n",
	)
	for _, unexpected := range []string{"12 345 678 901", "Сообщение вне лимита", "текст сообщения обрезан"} {
		if strings.Contains(content, unexpected) {
			t.Errorf("unexpected %q in request content:\n%s", unexpected, content)
		}
	}
}

// 2A-10, R2-1: лимит в рунах считается по однострочному замаскированному тексту — ровно по тому,
// что уходит провайдеру ("\r\n" превращается в один пробел, номер — в [телефон]).
func TestAnalyzeChat_ContextLimitCountsFlattenedText(t *testing.T) {
	t.Run("phone split by line break fits mask length", func(t *testing.T) {
		in := usecase.AnalyzeChatInput{Messages: []domain.Message{
			msg(1, domain.MessageDirectionIn, "+7 (999)\n123-45-67", baseTime),
		}}
		content := sendAndCaptureUserContent(t, len([]rune(phoneMask)), in)

		if !strings.Contains(content, "Клиент: [телефон]\n") {
			t.Errorf("masked phone must fit the limit intact; content:\n%s", content)
		}
		if strings.Contains(content, "обрезан") || strings.Contains(content, "999") {
			t.Errorf("message must be flattened and masked before the limit; content:\n%s", content)
		}
		assertNoPhoneLeftInContent(t, content)
	})

	t.Run("CRLF counts as one rune", func(t *testing.T) {
		in := usecase.AnalyzeChatInput{Messages: []domain.Message{
			msg(1, domain.MessageDirectionIn, "абв\r\nгде", baseTime), // 8 рун в исходнике, 7 после замены
		}}
		content := sendAndCaptureUserContent(t, 7, in)

		if !strings.Contains(content, "Клиент: абв где\n") {
			t.Errorf("flattened message must fit the limit intact; content:\n%s", content)
		}
		if strings.Contains(content, "обрезан") {
			t.Errorf("message must not be truncated; content:\n%s", content)
		}
	})
}

// isQ8SeparatorOracle — независимое от phoneCandidate перечисление разделителей Q8.
func isQ8SeparatorOracle(r rune) bool {
	switch r {
	case '+', '(', ')', '＋', '（', '）', '\t', '\v', '\f', '\r', '\n', 0x85, 0x2028, 0x2029, '\u2212',
		'\u200B', '\u200C', '\u200D', '\u2060', '\uFEFF', '\u00AD':
		return true
	}
	return unicode.Is(unicode.Zs, r) || unicode.Is(unicode.Pd, r)
}

// assertNoPhoneLeft проверяет независимо от phoneCandidate, что после удаления разделителей Q8
// в тексте не осталось ни одной подпоследовательности из 10+ цифр \p{Nd} подряд.
func assertNoPhoneLeft(t *testing.T, s string) {
	t.Helper()
	run := 0
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			run++
			if run >= minPhoneDigits {
				t.Errorf("phone-like digit run of %d+ digits left in %q", minPhoneDigits, s)
				return
			}
		case isQ8SeparatorOracle(r):
		default:
			run = 0
		}
	}
}

type maskCase struct {
	name, in, want string
}

func runMaskCases(t *testing.T, tests []maskCase, checkOracle bool) {
	t.Helper()
	for _, tt := range tests {
		name := tt.name
		if name == "" {
			name = tt.in
		}
		t.Run(name, func(t *testing.T) {
			got := maskPhones(tt.in)
			if got != tt.want {
				t.Errorf("maskPhones(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if checkOracle {
				assertNoPhoneLeft(t, got)
			}
		})
	}
}

// Q8: граничные случаи маскирования (ASCII).
func TestMaskPhones(t *testing.T) {
	runMaskCases(t, []maskCase{
		{"", "+7 (999) 123-45-67", "[телефон]"},
		{"", "89991234567", "[телефон]"},
		{"", "8-999-123-45-67", "[телефон]"},
		{"", "(999)1234567", "[телефон]"},                            // 10 цифр
		{"", "1234567890", "[телефон]"},                              // 10 цифр без разделителей
		{"", "+44 20 7946 0958", "[телефон]"},                        // 12 цифр
		{"", "123456789012345", "[телефон]"},                         // 15 цифр
		{"NBSP", "8\u00A0999\u00A0123\u00A045\u00A067", "[телефон]"}, // неразрывные пробелы
		{"", "tel:+79991234567.", "tel:[телефон]."},
		{"", "+7 - (999) - 123-45-67", "[телефон]"}, // 4 разделителя подряд не разрывают номер
		{"", "12345 67890", "[телефон]"},            // 10 цифр через пробел
		// 16+ цифр маскируются целиком (privacy-first): несколько номеров подряд и номер рядом с числом.
		{"", "1234567890123456", "[телефон]"},
		{"", "89991234567 89997654321", "[телефон]"},
		{"", "+7 999 123 45 67 +7 999 765 43 21", "[телефон]"},
		{"", "заказ 452187 89991234567", "заказ [телефон]"},
		{"", "ИНН 7701234567 89991234567", "ИНН [телефон]"},
		// Короткие числа и цепочки, разорванные не-разделителями, не меняются.
		{"", "42", "42"},
		{"", "12345", "12345"},
		{"", "123456789", "123456789"}, // 9 цифр
		{"", "заказ 42 на 3 шт", "заказ 42 на 3 шт"},
		{"", "2026-03-10", "2026-03-10"},
		{"", "10:30", "10:30"},
		{"", "встреча 2026-03-10 в 10:30", "встреча 2026-03-10 в 10:30"},
		{"", "12345.67890", "12345.67890"},
		{"", "12345/67890", "12345/67890"},
		{"", "12345, 67890", "12345, 67890"},
		{"", "12345:67890", "12345:67890"},
		{"", "нет цифр", "нет цифр"},
		{"", "", ""},
	}, true)
}

// Q8: Unicode-цифры, тире, пробелы, невидимые символы и переводы строк внутри номера.
func TestMaskPhones_Q8Unicode(t *testing.T) {
	runMaskCases(t, []maskCase{
		// Тире \p{Pd} и минус.
		{"non-breaking hyphen U+2011", "8\u2011999\u2011123\u201145\u201167", "[телефон]"},
		{"en dash U+2013", "8\u2013999\u2013123\u201345\u201367", "[телефон]"},
		{"em dash U+2014", "8\u2014999\u2014123\u201445\u201467", "[телефон]"},
		{"minus U+2212", "8\u2212999\u2212123\u221245\u221267", "[телефон]"},
		{"hyphen U+2010 and figure dash U+2012", "8\u2010999\u2012123-45-67", "[телефон]"},
		// Пробелы \p{Zs}.
		{"narrow NBSP U+202F", "+7\u202F999\u202F123\u202F45\u202F67", "[телефон]"},
		{"thin space U+2009", "8\u2009999\u2009123\u200945\u200967", "[телефон]"},
		{"figure space U+2007", "8\u2007999\u2007123\u200745\u200767", "[телефон]"},
		{"ideographic space U+3000", "8\u3000999\u30001234567", "[телефон]"},
		{"tab", "8\t999\t123\t45\t67", "[телефон]"},
		// Цифры \p{Nd} и полноширинные скобки/плюс.
		{"fullwidth digits", "８９１６１２３４５６７", "[телефон]"},
		{"fullwidth plus and parens", "＋７（９９９）１２３－４５－６７", "[телефон]"},
		{"fullwidth leading paren", "（９９９）１２３４５６７", "[телефон]"},
		{"mixed fullwidth and ASCII", "+7 (９９９) 123-45-67", "[телефон]"},
		{"arabic-indic digits", "٨٩١٦١٢٣٤٥٦٧", "[телефон]"},
		{"fullwidth in text", "Звоните ８９１６\u2011１２３\u2011４５\u2011６７ вечером", "Звоните [телефон] вечером"},
		// Невидимые символы.
		{"zero width space U+200B", "8916\u200B123\u200B45\u200B67", "[телефон]"},
		{"ZWNJ U+200C and ZWJ U+200D", "8916\u200C123\u200D4567", "[телефон]"},
		{"word joiner U+2060", "8916\u20601234567", "[телефон]"},
		{"BOM U+FEFF", "\uFEFF8916\uFEFF1234567", "\uFEFF[телефон]"},
		{"soft hyphen U+00AD", "8916\u00AD123\u00AD45\u00AD67", "[телефон]"},
		// Переводы строк: maskPhones безопасна и без flattenLines.
		{"LF", "8 999\n123 45 67", "[телефон]"},
		{"CRLF", "+7 (999)\r\n765-43-21", "[телефон]"},
		{"CR", "8999\r1112233", "[телефон]"},
		{"LF in text", "тел.\n8 999\n123 45 67\nспасибо", "тел.\n[телефон]\nспасибо"},
		{"VT and FF", "8\v999\f123 45 67", "[телефон]"},
		{"NEL U+0085", "8 999\u0085123 45 67", "[телефон]"},
		{"line separator U+2028", "8 999\u2028123 45 67", "[телефон]"},
		{"paragraph separator U+2029", "8 999\u2029123\u202945 67", "[телефон]"},
		// Короткие Unicode-цепочки и разрывающие символы.
		{"short fullwidth", "１２３４５", "１２３４５"},
		{"short with U+2011", "8\u2011999", "8\u2011999"},
		{"9 fullwidth digits", "１２３\u2011４５６\u2011７８９", "１２３\u2011４５６\u2011７８９"},
		{"middle dot breaks", "12345·67890", "12345·67890"},
		{"fullwidth full stop breaks", "12345．67890", "12345．67890"},
	}, true)
}

// Q8: исключения «ISO-дата в начале цепочки» и «денежная сумма с маркером» и их отрицательные
// случаи. Независимый оракул не применяется: сохранённые даты и суммы содержат 10+ цифр.
func TestMaskPhones_Q8Exceptions(t *testing.T) {
	runMaskCases(t, []maskCase{
		// Дата в начале цепочки, после неё конец цепочки или пробел.
		{"date and time", "2026-03-10 10:30", "2026-03-10 10:30"},
		{"date and time in text", "встреча 2026-03-10 10:30 в офисе", "встреча 2026-03-10 10:30 в офисе"},
		{"date NBSP time", "2026-03-10\u00A010:30", "2026-03-10\u00A010:30"},
		{"date LF time", "2026-03-10\n10:30", "2026-03-10\n10:30"},
		{"date with en dashes", "2026\u201303\u201310 10:30", "2026\u201303\u201310 10:30"},
		{"date with minus signs", "1999\u221212\u221231 23:59", "1999\u221212\u221231 23:59"},
		{"two dates", "2026-03-10 2026-03-11 10:30", "2026-03-10 2026-03-11 10:30"},
		{"date range by space", "2026-03-10 11 12", "2026-03-10 11 12"},
		// Дата сохраняется, остаток проверяется отдельно.
		{"date and phone", "2026-03-10 89161234567", "2026-03-10 [телефон]"},
		{"date and plus phone", "2026-03-10 +7 999 123-45-67", "2026-03-10 [телефон]"},
		{"date LF phone", "2026-03-10\n8 999 123 45 67", "2026-03-10\n[телефон]"},
		{"date U+2028 phone", "2026-03-10\u2028+7 999 123 45 67", "2026-03-10\u2028[телефон]"},
		{"date paren phone", "2026-03-10) 89161234567", "2026-03-10) [телефон]"},
		{"date fullwidth paren phone", "2026-03-10）89161234567", "2026-03-10）[телефон]"},
		{"date paren plus phone", "2026-03-10) +7 999 123-45-67", "2026-03-10) [телефон]"},
		{"date in parens and time", "(2026-03-10) 10:30", "(2026-03-10) 10:30"},
		{"date paren then date", "2026-03-10) (2026-03-11 10:30)", "2026-03-10) (2026-03-11 10:30)"},
		{"date time phone", "2026-03-10 10 89161234567", "2026-03-10 [телефон]"},
		{"two dates and phone", "2026-03-10 2026-03-11 89161234567", "2026-03-10 2026-03-11 [телефон]"},
		// Исключение даты не применяется.
		{"dash after date", "2026-03-10-35 1234", "[телефон]"},
		{"digit right after date", "2026-03-1012345678", "[телефон]"},
		{"invisible after date", "2026-03-10\u200B10:30", "[телефон]:30"},
		{"chain starts before date", "+7 2026-03-10 11", "[телефон]"},
		{"digits before date", "8 2026-03-10 11", "[телефон]"},
		// Ведущая "(" или "（" частью номера не считается; ведущий "+"/"＋" исключение отменяет.
		{"leading paren before date", "(2026-03-10 10:30)", "(2026-03-10 10:30)"},
		{"leading fullwidth paren before date", "（2026-03-10 10:30）", "（2026-03-10 10:30）"},
		{"leading paren date and phone", "(2026-03-10 89161234567)", "(2026-03-10 [телефон])"},
		{"leading paren dash after date", "(2026-03-10-35 1234)", "[телефон])"},
		{"leading plus before date", "+2026-03-10 10", "[телефон]"},
		{"leading fullwidth plus before date", "＋2026-03-10 10", "[телефон]"},
		{"paren area code is not a date", "(495) 123-45-67", "[телефон]"},
		{"paren before invalid date", "(2026-13-10 10)", "[телефон])"},
		{"invalid month", "2026-13-10 10", "[телефон]"},
		{"month 00", "2026-00-10 10", "[телефон]"},
		{"invalid day", "2026-03-32 10", "[телефон]"},
		{"day 00", "2026-03-00 10", "[телефон]"},
		{"invalid century", "1826-03-10 10", "[телефон]"},
		{"fullwidth date digits", "２０２６-０３-１０ １０:３０", "[телефон]:３０"},
		// Денежная сумма с маркером.
		{"sum руб", "1 500 000 000 руб", "1 500 000 000 руб"},
		{"sum ₽", "1 500 000 000 ₽", "1 500 000 000 ₽"},
		{"sum ₽ no space", "1 500 000 000₽", "1 500 000 000₽"},
		{"dollar before sum", "$ 1 500 000 000", "$ 1 500 000 000"},
		{"dollar before sum no space", "$1 500 000 000", "$1 500 000 000"},
		{"euro before sum", "сумма €1 500 000 000.", "сумма €1 500 000 000."},
		{"ruble sign before sum", "₽ 1 500 000 000", "₽ 1 500 000 000"},
		{"upper case РУБ", "1 500 000 000 РУБ", "1 500 000 000 РУБ"},
		{"рублей", "итого 1 500 000 000 рублей", "итого 1 500 000 000 рублей"},
		{"р.", "1 500 000 000 р.", "1 500 000 000 р."},
		{"USD", "1 500 000 000 USD", "1 500 000 000 USD"},
		{"rub", "1 500 000 000 rub", "1 500 000 000 rub"},
		{"Eur", "1 500 000 000 Eur", "1 500 000 000 Eur"},
		{"тыс", "12 500 000 000 тыс.", "12 500 000 000 тыс."},
		{"млн", "1 500 000 000 млн", "1 500 000 000 млн"},
		{"МЛРД", "1 500 000 000 МЛРД", "1 500 000 000 МЛРД"},
		{"$ after", "1 500 000 000 $", "1 500 000 000 $"},
		{"€ after", "1 500 000 000€", "1 500 000 000€"},
		{"NBSP groups", "1\u00A0500\u00A0000\u00A0000\u00A0руб", "1\u00A0500\u00A0000\u00A0000\u00A0руб"},
		{"narrow NBSP groups", "1\u202F500\u202F000\u202F000 ₽", "1\u202F500\u202F000\u202F000 ₽"},
		// Отрицательные случаи суммы.
		{"sum without marker", "1 500 000 000", "[телефон]"},
		{"wrong grouping", "15 000 000 000 00 руб", "[телефон] руб"},
		{"leading group of 4", "1500 000 000 руб", "[телефон] руб"},
		{"dash grouping", "1-500-000-000 руб", "[телефон] руб"},
		{"two spaces between groups", "1 500  000 000 руб", "[телефон] руб"},
		{"two spaces before marker", "1 500 000 000  руб", "[телефон]  руб"},
		{"two spaces after dollar", "$  1 500 000 000", "$  [телефон]"},
		{"р without dot", "1 500 000 000 р", "[телефон] р"},
		{"unrelated word", "1 500 000 000 рынок", "[телефон] рынок"},
		{"plus before sum", "+1 500 000 000 $", "[телефон] $"},
		{"marker far away", "1 500 000 000 примерно руб", "[телефон] примерно руб"},
		{"fullwidth sum digits", "１ ５００ ０００ ０００ руб", "[телефон] руб"},
		{"phone shaped as groups", "+7 916 123 456 7890", "[телефон]"},
	}, false)
}

// Q8, F1: длинная цепочка дат (~550 KB) обрабатывается за линейное время. При квадратичной
// регрессии (пересчёт цифр и поиск по всему остатку на каждой дате) тест выполняется на порядки
// дольше и упирается в таймаут go test; жёстких проверок времени нет.
func TestMaskPhones_LongDateChainIsLinear(t *testing.T) {
	const repeats = 50000
	dates := strings.Repeat("2026-03-10 ", repeats-1) + "2026-03-10"

	if got := maskPhones(dates); got != dates {
		t.Errorf("chain of %d dates must stay unchanged (len got %d, want %d)", repeats, len(got), len(dates))
	}

	withPhone := dates + " 89161234567 и (2026-03-11) 10:30"
	want := dates + " [телефон] и (2026-03-11) 10:30"
	if got := maskPhones(withPhone); got != want {
		t.Errorf("phone after a chain of %d dates must be masked; got suffix %q", repeats, got[max(0, len(got)-40):])
	}
}

// 2A-10, Q8: Unicode-варианты номеров маскируются в контексте, уходящем провайдеру.
func TestAnalyzeChat_ContextMasksUnicodePhones(t *testing.T) {
	in := usecase.AnalyzeChatInput{Messages: []domain.Message{
		msg(1, domain.MessageDirectionIn, "Звоните ８９１６\u2011１２３\u2011４５\u2011６７ или 8\u2013999\u2014765\u221243\u201121", baseTime),
		msg(2, domain.MessageDirectionOut, "Мой 8\u00A0999\u202F555\u200966\u200B77, встреча 2026-03-10 10:30, бюджет 1 500 000 000 руб", baseTime.Add(time.Minute)),
		msg(3, domain.MessageDirectionIn, "Запасной ＋７（９９９）１１１\u00AD２２\u206033", baseTime.Add(2*time.Minute)),
	}}
	in.Previous = &domain.ChatSummary{
		ChatID:        7,
		NextStep:      "Перезвонить 2026-03-10 на 8\u2011999\u2011444\u201155\u201166",
		Risk:          domain.RiskLow,
		LastMessageID: 1,
	}
	content := sendAndCaptureUserContent(t, testMaxChars, in)

	assertInOrder(t, content,
		"- next_step: Перезвонить 2026-03-10 на [телефон]\n",
		"Клиент: Звоните [телефон] или [телефон]\n",
		"Менеджер: Мой [телефон], встреча 2026-03-10 10:30, бюджет 1 500 000 000 руб\n",
		"Клиент: Запасной [телефон]\n",
	)
	for _, leaked := range []string{"９１６", "999", "765", "555", "１１１", "444"} {
		if strings.Contains(content, leaked) {
			t.Errorf("phone fragment %q leaked into request; content:\n%s", leaked, content)
		}
	}
}
