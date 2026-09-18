#!/bin/bash

# Script to generate OpenAPI specification from code

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
SPEC_FILE="$PROJECT_ROOT/docs/api/openapi.yaml"

echo "=== OpenAPI Specification Generation ==="
echo "Output file: $SPEC_FILE"
echo

# Ensure docs/api directory exists
mkdir -p "$PROJECT_ROOT/docs/api"

# Check if swag is installed
if ! command -v swag &> /dev/null; then
    echo "📦 Installing swag..."
    go install github.com/swaggo/swag/cmd/swag@latest
fi

# Generate OpenAPI from swagger annotations
echo "🔨 Generating OpenAPI spec from code annotations..."
cd "$PROJECT_ROOT"
swag init \
    --generalInfo cmd/api/main.go \
    --dir ./ \
    --output docs/api \
    --outputTypes yaml,json \
    --parseDependency \
    --parseInternal \
    --parseDepth 3

# Post-process the generated spec
echo "🔧 Post-processing OpenAPI spec..."
python3 - <<'PYTHON_SCRIPT'
import yaml
import sys
from pathlib import Path

spec_file = Path(sys.argv[1])
if not spec_file.exists():
    print(f"Error: {spec_file} not found")
    sys.exit(1)

with open(spec_file, 'r') as f:
    spec = yaml.safe_load(f)

# Add additional metadata
if 'info' in spec:
    spec['info']['x-api-id'] = 'donelist-api'
    spec['info']['x-audience'] = 'external-public'

# Add rate limit headers to all endpoints
if 'paths' in spec:
    for path, methods in spec['paths'].items():
        for method, operation in methods.items():
            if method in ['get', 'post', 'put', 'patch', 'delete']:
                if 'responses' not in operation:
                    operation['responses'] = {}

                # Add rate limit response
                if '429' not in operation['responses']:
                    operation['responses']['429'] = {
                        'description': 'Rate limit exceeded',
                        'content': {
                            'application/json': {
                                'schema': {
                                    '$ref': '#/components/schemas/ErrorResponse'
                                }
                            }
                        },
                        'headers': {
                            'X-RateLimit-Limit': {
                                'schema': {'type': 'integer'},
                                'description': 'Request limit per hour'
                            },
                            'X-RateLimit-Remaining': {
                                'schema': {'type': 'integer'},
                                'description': 'Requests remaining in current window'
                            },
                            'X-RateLimit-Reset': {
                                'schema': {'type': 'integer'},
                                'description': 'Time when rate limit resets (Unix timestamp)'
                            }
                        }
                    }

# Write back the modified spec
with open(spec_file, 'w') as f:
    yaml.dump(spec, f, default_flow_style=False, sort_keys=False)

print(f"✅ Post-processing complete: {spec_file}")
PYTHON_SCRIPT

python3 - "$SPEC_FILE"

# Validate the generated spec
echo
echo "🔍 Validating generated spec..."
"$SCRIPT_DIR/validate-openapi.sh"

# Generate TypeScript types (if ts-morph is available)
if command -v openapi-typescript &> /dev/null; then
    echo
    echo "📘 Generating TypeScript types..."
    openapi-typescript "$SPEC_FILE" --output "$PROJECT_ROOT/docs/api/types.ts"
fi

# Generate Go client (optional)
if command -v oapi-codegen &> /dev/null; then
    echo
    echo "🔷 Generating Go client types..."
    oapi-codegen -package api -generate types "$SPEC_FILE" > "$PROJECT_ROOT/internal/api/generated_types.go"
fi

echo
echo "✅ OpenAPI specification generated successfully!"
echo "   📄 Spec: $SPEC_FILE"
echo "   📊 Stats:"
echo "      - Endpoints: $(grep -c '^ {2}/' "$SPEC_FILE" || echo 0)"
echo "      - Schemas: $(grep -c '^  \w\+:$' "$SPEC_FILE" || echo 0)"
