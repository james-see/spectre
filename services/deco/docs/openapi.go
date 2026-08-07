package docs

// OpenAPI is a minimal OpenAPI 3 document for the Deco LAN poller.
var OpenAPI = []byte(`{
  "openapi": "3.0.3",
  "info": {
    "title": "Spectre Deco LAN API",
    "version": "1.0.0",
    "description": "Local TP-Link Deco mesh + client poller for Spectre Observatory."
  },
  "paths": {
    "/health": {
      "get": {
        "summary": "Health",
        "responses": {
          "200": {
            "description": "Service status",
            "content": {
              "application/json": {
                "schema": { "type": "object" }
              }
            }
          }
        }
      }
    },
    "/api/lan/mesh": {
      "get": {
        "summary": "Deco mesh nodes",
        "description": "List TP-Link Deco APs (master + satellites).",
        "responses": {
          "200": {
            "description": "Mesh snapshot",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "mesh": {
                      "type": "array",
                      "items": { "$ref": "#/components/schemas/MeshNode" }
                    },
                    "error": { "type": "string" },
                    "updated_at": { "type": "string", "format": "date-time" }
                  }
                }
              }
            }
          }
        }
      }
    },
    "/api/lan/clients": {
      "get": {
        "summary": "Wi-Fi / LAN clients",
        "description": "Associated stations; Spectre ESP32 MACs (7c:4f:ad) excluded.",
        "responses": {
          "200": {
            "description": "Client snapshot",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "clients": {
                      "type": "array",
                      "items": { "$ref": "#/components/schemas/Client" }
                    },
                    "error": { "type": "string" },
                    "updated_at": { "type": "string", "format": "date-time" }
                  }
                }
              }
            }
          }
        }
      }
    }
  },
  "components": {
    "schemas": {
      "MeshNode": {
        "type": "object",
        "properties": {
          "id": { "type": "string" },
          "name": { "type": "string" },
          "mac": { "type": "string" },
          "ip": { "type": "string" },
          "role": { "type": "string", "enum": ["master", "satellite"] },
          "online": { "type": "boolean" }
        }
      },
      "Client": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "mac": { "type": "string" },
          "ip": { "type": "string" },
          "online": { "type": "boolean" },
          "band": { "type": "string" },
          "connection_type": { "type": "string" },
          "deco_mac": { "type": "string" },
          "deco_name": { "type": "string" },
          "up_rate_kbps": { "type": "integer" },
          "down_rate_kbps": { "type": "integer" }
        }
      }
    }
  }
}
`)
