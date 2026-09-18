#!/bin/bash

# Script to validate OpenAPI specification

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
SPEC_FILE="$PROJECT_ROOT/docs/api/openapi.yaml"

echo "=== OpenAPI Specification Validation ==="
echo "Spec file: $SPEC_FILE"
echo

# Check if spec file exists
if [ ! -f "$SPEC_FILE" ]; then
    echo "❌ OpenAPI spec file not found: $SPEC_FILE"
    exit 1
fi

# Install spectral if not installed
if ! command -v spectral &> /dev/null; then
    echo "📦 Installing Spectral CLI..."
    npm install -g @stoplight/spectral-cli
fi

# Create Spectral ruleset if it doesn't exist
SPECTRAL_CONFIG="$PROJECT_ROOT/.spectral.yml"
if [ ! -f "$SPECTRAL_CONFIG" ]; then
    echo "📝 Creating Spectral configuration..."
    cat > "$SPECTRAL_CONFIG" << 'EOF'
extends: ["spectral:oas"]
rules:
  # Enable all recommended rules
  operation-operationId: error
  operation-parameters: error
  operation-tag-defined: error
  path-params: error

  # API design rules
  operation-description: warn
  operation-success-response: error
  oas3-request-body-optional-true: warn

  # Security rules
  oas3-operation-security-defined: error

  # Schema rules
  oas3-schema: error
  typed-enum: warn

  # Custom rules for our API
  path-keys-no-trailing-slash:
    description: Path keys should not have trailing slashes
    given: $.paths[*]~
    severity: error
    then:
      function: pattern
      functionOptions:
        notMatch: "/$"

  response-error-structure:
    description: Error responses must follow our structure
    given: $.paths[*][*].responses[?(@property >= '400')].content['application/json'].schema
    severity: warn
    then:
      function: schema
      functionOptions:
        schema:
          type: object
          required: ["$ref"]
EOF
fi

# Run Spectral linting
echo "🔍 Running Spectral linting..."
if spectral lint "$SPEC_FILE" --format pretty; then
    echo "✅ OpenAPI spec validation passed!"
else
    echo "❌ OpenAPI spec validation failed!"
    exit 1
fi

# Validate with OpenAPI parser
echo
echo "🔍 Validating OpenAPI structure..."
if command -v swagger-cli &> /dev/null; then
    swagger-cli validate "$SPEC_FILE"
else
    echo "⚠️  swagger-cli not found, skipping structural validation"
    echo "   Install with: npm install -g @apidevtools/swagger-cli"
fi

# Generate statistics
echo
echo "📊 OpenAPI Specification Statistics:"
echo "-----------------------------------"
echo "Endpoints: $(grep -c '^ {2}/' "$SPEC_FILE" || echo 0)"
echo "Schemas: $(grep -c '^    \w\+:$' "$SPEC_FILE" | head -1 || echo 0)"
echo "Parameters: $(grep -c 'in: ' "$SPEC_FILE" || echo 0)"

echo
echo "✅ All validation checks completed!"