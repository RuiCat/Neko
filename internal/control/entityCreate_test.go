// entityCreate_test.go covers POST /entities/create: template creation,
// idempotency for an existing entity, persistence and the 400/405 vocabulary.
package control

import (
	"net/http"
	"testing"
)

// TestEntityCreateAddsEntity covers the happy path: a new entity is created
// from the template chain and persisted.
func TestEntityCreateAddsEntity(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodPost, "/entities/create", []byte(`{"entity":"梦梦","type":"NPC","data":{"备注":"新实体"}}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["ok"] != true || payload["entity"] != "梦梦" || payload["created"] != true {
		t.Fatalf("payload = %v", payload)
	}
	created := env.runtime.Store.Get("梦梦")
	if created == nil {
		t.Fatal("entity not in the store")
	}
	if created.Type != "NPC" {
		t.Fatalf("type = %q", created.Type)
	}
	if recorder := env.call(t, http.MethodGet, "/entities", nil); recorder.Code != http.StatusOK {
		t.Fatalf("entities status = %d", recorder.Code)
	}
}

// TestEntityCreateIsIdempotent keeps an existing entity untouched and reports
// created=false instead of a validation error.
func TestEntityCreateIsIdempotent(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodPost, "/entities/create", []byte(`{"entity":"猫猫","type":"NPC"}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["created"] != false {
		t.Fatalf("payload = %v", payload)
	}
	if env.runtime.Store.Get("猫猫") == nil {
		t.Fatal("existing entity disappeared")
	}
}

// TestEntityCreateRejectsBadRequests covers the request vocabulary: method,
// body, missing id and an unknown template type.
func TestEntityCreateRejectsBadRequests(t *testing.T) {
	env := newTestEnv(t, Options{})
	if recorder := env.call(t, http.MethodGet, "/entities/create", nil); recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d", recorder.Code)
	}
	for _, body := range []string{``, `not-json`, `{}`, `{"entity":"   "}`, `{"entity":"梦梦","type":"不存在"}`} {
		recorder := env.call(t, http.MethodPost, "/entities/create", []byte(body))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %q status = %d (%s)", body, recorder.Code, recorder.Body.String())
		}
	}
}
