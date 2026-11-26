package transporthttp

import (
	"net/http"

	"recsys/internal/transport/http/respond"
)

const swaggerIndexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Softline Recommendations API — Swagger</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function () {
      window.ui = SwaggerUIBundle({
        url: "/swagger/openapi.yaml",
        dom_id: "#swagger-ui",
        deepLinking: true
      });
    };
  </script>
</body>
</html>
`

func (s *Server) handleGetSwagger(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/swagger" {
		http.Redirect(w, r, "/swagger/", http.StatusMovedPermanently)
		return
	}
	if r.URL.Path != "/swagger/" {
		respond.Error(w, r, http.StatusNotFound, "not_found", "Swagger resource not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerIndexHTML))
}

func (s *Server) handleGetSwaggerSpec(w http.ResponseWriter, r *http.Request) {
	if len(s.openAPISpec) == 0 {
		respond.Error(w, r, http.StatusNotFound, "openapi_not_found", "OpenAPI specification is not loaded")
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.openAPISpec)
}
