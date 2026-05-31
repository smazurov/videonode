package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/smazurov/videonode/internal/api/models"
)

// composerSvcStub is a minimal ComposerService mock exercising only the
// export/import routes; the other methods satisfy the interface.
type composerSvcStub struct {
	exportData []byte
	exportErr  error

	imported    []byte
	importData  *models.ComposerData
	importCreat bool
	importErr   error

	intoID   string
	intoData []byte
	intoResp *models.ComposerData
	intoErr  error
}

func (m *composerSvcStub) ListComposers(context.Context) ([]models.ComposerData, error) {
	return nil, nil
}

func (m *composerSvcStub) GetComposer(context.Context, string) (*models.ComposerData, error) {
	return nil, nil
}

func (m *composerSvcStub) CreateComposer(context.Context, models.ComposerCreateRequestData) (*models.ComposerData, error) {
	return nil, nil
}

func (m *composerSvcStub) UpdateComposer(context.Context, string, models.ComposerUpdateRequestData) (*models.ComposerData, error) {
	return nil, nil
}
func (m *composerSvcStub) DeleteComposer(context.Context, string) error { return nil }
func (m *composerSvcStub) ReplaceLayout(context.Context, string, []models.LayoutSlotData) (*models.ComposerData, error) {
	return nil, nil
}

func (m *composerSvcStub) SetInputEffect(context.Context, string, string, *models.EffectData) (*models.ComposerData, error) {
	return nil, nil
}

func (m *composerSvcStub) ExportComposer(_ context.Context, _ string) ([]byte, error) {
	return m.exportData, m.exportErr
}

func (m *composerSvcStub) ImportComposer(_ context.Context, data []byte) (*models.ComposerData, bool, error) {
	m.imported = data
	return m.importData, m.importCreat, m.importErr
}

func (m *composerSvcStub) ImportComposerInto(_ context.Context, id string, data []byte) (*models.ComposerData, error) {
	m.intoID = id
	m.intoData = data
	return m.intoResp, m.intoErr
}

func newComposerTestServer(t *testing.T, svc ComposerService) humatest.TestAPI {
	t.Helper()
	_, papi := humatest.New(t)
	s := &Server{api: papi, composerService: svc}
	s.registerComposerRoutes()
	return papi
}

func TestExportComposerRoute_ServesTOML(t *testing.T) {
	doc := "id = 'main'\n[canvas]\nw = 1920\nh = 1080\n"
	api := newComposerTestServer(t, &composerSvcStub{exportData: []byte(doc)})

	resp := api.Get("/api/composers/main/export")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s; want 200", resp.Code, resp.Body.String())
	}
	if ct := resp.Header().Get("Content-Type"); ct != "application/toml" {
		t.Errorf("Content-Type = %q; want application/toml", ct)
	}
	if cd := resp.Header().Get("Content-Disposition"); !strings.Contains(cd, `main.toml`) {
		t.Errorf("Content-Disposition = %q; want it to name main.toml", cd)
	}
	if resp.Body.String() != doc {
		t.Errorf("body = %q; want the raw TOML %q", resp.Body.String(), doc)
	}
}

func TestExportComposerRoute_NotFoundMapsTo404(t *testing.T) {
	api := newComposerTestServer(t, &composerSvcStub{
		exportErr: &ComposerError{Code: ComposerErrNotFound, Message: "nope"},
	})
	if resp := api.Get("/api/composers/ghost/export"); resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", resp.Code)
	}
}

func TestImportComposerRoute_PassesRawBodyAndReturnsComposer(t *testing.T) {
	doc := "id = 'main'\n[canvas]\nw = 1920\nh = 1080\n[[inputs]]\nref = 'source:cam'\n"
	svc := &composerSvcStub{
		importData:  &models.ComposerData{ID: "main"},
		importCreat: true,
	}
	api := newComposerTestServer(t, svc)

	resp := api.Post("/api/composers/import",
		"Content-Type: application/toml",
		strings.NewReader(doc),
	)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s; want 200", resp.Code, resp.Body.String())
	}
	if string(svc.imported) != doc {
		t.Errorf("service received body %q; want raw TOML %q", svc.imported, doc)
	}
	if !strings.Contains(resp.Body.String(), `"id":"main"`) {
		t.Errorf("response body = %s; want the imported composer JSON", resp.Body.String())
	}
}

func TestImportComposerIntoRoute_PassesPathIDAndRawBody(t *testing.T) {
	doc := "id = 'ignored'\n[canvas]\nw = 1920\nh = 1080\n[[inputs]]\nref = 'source:cam'\n"
	svc := &composerSvcStub{intoResp: &models.ComposerData{ID: "dest"}}
	api := newComposerTestServer(t, svc)

	resp := api.Post("/api/composers/dest/import",
		"Content-Type: application/toml",
		strings.NewReader(doc),
	)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s; want 200", resp.Code, resp.Body.String())
	}
	if svc.intoID != "dest" {
		t.Errorf("service got id %q; want dest", svc.intoID)
	}
	if string(svc.intoData) != doc {
		t.Errorf("service received body %q; want raw TOML %q", svc.intoData, doc)
	}
	if !strings.Contains(resp.Body.String(), `"id":"dest"`) {
		t.Errorf("response body = %s; want the imported composer JSON", resp.Body.String())
	}
}

func TestImportComposerIntoRoute_NotFoundMapsTo404(t *testing.T) {
	svc := &composerSvcStub{intoErr: &ComposerError{Code: ComposerErrNotFound, Message: "nope"}}
	api := newComposerTestServer(t, svc)
	resp := api.Post("/api/composers/ghost/import",
		"Content-Type: application/toml",
		strings.NewReader("id='x'\n[canvas]\nw=1\nh=1\n[[inputs]]\nref='source:a'\n"),
	)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", resp.Code)
	}
}

func TestImportComposerRoute_InvalidMapsTo400(t *testing.T) {
	svc := &composerSvcStub{importErr: &ComposerError{Code: ComposerErrInvalid, Message: "bad toml"}}
	api := newComposerTestServer(t, svc)

	resp := api.Post("/api/composers/import",
		"Content-Type: application/toml",
		strings.NewReader("not valid"),
	)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s; want 400", resp.Code, resp.Body.String())
	}
}
