package web

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndexLinksWhatsAppPage(t *testing.T) {
	srv := newTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.srv.Handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if !strings.Contains(body, `href="/whatsapp"`) {
		t.Fatal("expected index page to link to WhatsApp message sending page")
	}
	if !strings.Contains(body, "WhatsApp 信息发送") {
		t.Fatal("expected index navigation to include WhatsApp 信息发送")
	}
}

func TestWhatsAppPageUsesNamespacedAssetsAndAPI(t *testing.T) {
	srv := newTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatsapp", nil)
	srv.srv.Handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	for _, expected := range []string{
		`href="/static/whatsapp/style.css"`,
		`src="/static/whatsapp/app.js"`,
		`const apiBase = "/whatsapp/api";`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected WhatsApp page to contain %q", expected)
		}
	}
}

func TestWhatsAppLoginStatusIsServedByGo(t *testing.T) {
	srv := newTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatsapp/api/login/status", nil)
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"logged_in":false,"phone":null}` {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestWhatsAppWebLoggedInSelectorsCoverChatListWithoutOpenChat(t *testing.T) {
	selectors := strings.Join(whatsAppSelectors["logged_in"], "\n")

	for _, expected := range []string{"#side", "Chat list", "聊天列表"} {
		if !strings.Contains(selectors, expected) {
			t.Fatalf("logged in selectors do not include %q", expected)
		}
	}
}

func TestWhatsAppWebInvalidPhonePopupSelectorsCoverChineseAndEnglishPrompts(t *testing.T) {
	selectors := strings.Join(whatsAppSelectors["invalid_phone_popup"], "\n")

	for _, expected := range []string{"未注册", "无效", "not registered", "invalid"} {
		if !strings.Contains(selectors, expected) {
			t.Fatalf("invalid phone popup selectors do not include %q", expected)
		}
	}
}

func TestWhatsAppContactsUploadParsesScraperCSV(t *testing.T) {
	srv := newTestServer(t)
	body, contentType := multipartBody(t, "file", "contacts.csv", "商家名称,电话,地址,分类,评分,邮箱,官网\n店铺A,+86 138-0013-8000,上海,餐厅,4.8,a@example.com,https://example.com\n")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/whatsapp/api/contacts/upload", body)
	req.Header.Set("Content-Type", contentType)
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	for _, expected := range []string{`"total":1`, `"shop_name":"店铺A"`, `"phone":"8613800138000"`} {
		if !strings.Contains(rec.Body.String(), expected) {
			t.Fatalf("expected body to contain %q, got %s", expected, rec.Body.String())
		}
	}
}

func TestWhatsAppSendRequiresLogin(t *testing.T) {
	srv := newTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/whatsapp/api/send", strings.NewReader(`{"contact_ids":["1"],"messages":[{"text":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "请先登录 WhatsApp") {
		t.Fatalf("expected login error, got %s", rec.Body.String())
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	srv, err := New(NewService(nil, t.TempDir()), ":0")
	if err != nil {
		t.Fatal(err)
	}

	return srv
}

func multipartBody(t *testing.T, fieldName, fileName, content string) (io.Reader, string) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(part, strings.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return &body, writer.FormDataContentType()
}
