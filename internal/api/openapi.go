package api

import (
	"net/http"
)

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(openAPISpec))
}

const openAPISpec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "Capper Control Plane API",
    "version": "1.0.0",
    "description": "REST API for Capper WebUI and automation"
  },
  "servers": [{"url": "/api/v1"}],
  "paths": {
    "/health": {"get": {"summary": "Health check"}},
    "/instances": {
      "get": {"summary": "List instances"},
      "post": {"summary": "Create instance (optional runtimeMode: auto|bwrap|chroot|crun|runc|lxc|qemu)"}
    },
    "/instances/{id}": {
      "get": {"summary": "Get instance"},
      "delete": {"summary": "Delete instance"}
    },
    "/instances/{id}/start": {"post": {"summary": "Start instance"}},
    "/instances/{id}/stop": {"post": {"summary": "Stop instance"}},
    "/instances/{id}/restart": {"post": {"summary": "Restart instance"}},
    "/instances/{id}/logs/stdout": {"get": {"summary": "Instance stdout logs"}},
    "/instances/{id}/logs/stderr": {"get": {"summary": "Instance stderr logs"}},
    "/instances/{id}/terminal": {"get": {"summary": "WebSocket terminal"}},
    "/images": {"get": {"summary": "List images"}},
    "/images/upload": {"post": {"summary": "Upload .cap image (multipart)"}},
    "/images/{name}": {"get": {"summary": "Get image"}, "delete": {"summary": "Delete image"}},
    "/images/{name}/scan": {"post": {"summary": "Posture scan image"}},
    "/images/{name}/sbom": {"get": {"summary": "Generate SBOM"}},
    "/images/{name}/provenance": {"get": {"summary": "Generate provenance"}},
    "/capsule-types": {"get": {"summary": "List capsule types"}, "post": {"summary": "Create capsule type"}},
    "/storage/buckets/{bucket}/objects": {"get": {"summary": "List objects"}},
    "/storage/buckets/{bucket}/objects/{key}": {
      "get": {"summary": "Download object"},
      "put": {"summary": "Upload object"},
      "post": {"summary": "Upload object (multipart)"},
      "delete": {"summary": "Delete object"}
    },
    "/iam/simulate": {"post": {"summary": "Simulate IAM policy"}},
    "/iam/tokens": {"get": {"summary": "List tokens"}, "post": {"summary": "Issue token"}},
    "/auth/session": {"post": {"summary": "Create browser session"}, "delete": {"summary": "Clear session"}},
    "/openapi.json": {"get": {"summary": "OpenAPI specification"}}
  },
  "components": {
    "securitySchemes": {
      "bearerAuth": {"type": "http", "scheme": "bearer"},
      "sessionCookie": {"type": "apiKey", "in": "cookie", "name": "capper_session"}
    }
  },
  "security": [{"bearerAuth": []}, {"sessionCookie": []}]
}`
