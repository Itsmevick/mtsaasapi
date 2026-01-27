package router

import (
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

func SetupOpenAPI(r *chi.Mux) {
	// Serve OpenAPI spec
	r.Get("/openapi.yaml", func(w http.ResponseWriter, req *http.Request) {
		// Try multiple paths for flexibility
		paths := []string{
			"openapi.yaml",
			"./openapi.yaml",
			"../openapi.yaml",
			"services/api/openapi.yaml",
			"./services/api/openapi.yaml",
		}
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				http.ServeFile(w, req, path)
				return
			}
		}
		http.Error(w, "OpenAPI spec not found", http.StatusNotFound)
	})

	// Serve Swagger UI (optional - would need to add swagger-ui assets)
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`
			<!DOCTYPE html>
			<html>
			<head>
				<title>API Documentation</title>
				<link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui.css" />
			</head>
			<body>
				<div id="swagger-ui"></div>
				<script src="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui-bundle.js"></script>
				<script>
					SwaggerUIBundle({
						url: "/openapi.yaml",
						dom_id: "#swagger-ui",
					});
				</script>
			</body>
			</html>
		`))
	})
}
