# Data Export System

## Overview

The data export system allows users to export their checkin data in multiple formats (CSV, JSON, PDF) with background job processing and automatic cleanup.

## Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ POST /api/v1/export/request
       │ {"format": "csv", "start_date": "...", "end_date": "..."}
       ▼
┌──────────────────┐
│  ExportHandler   │
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│  ExportService   │◄──────┐
└──────┬───────────┘       │
       │                   │
       ├─────► Create Job  │
       │                   │
       └─────► Process Async
                    │
                    ▼
            ┌───────────────┐
            │  Background   │
            │  Processing   │
            └───────┬───────┘
                    │
        ┌───────────┼───────────┐
        │           │           │
        ▼           ▼           ▼
   ┌────────┐  ┌─────────┐  ┌──────────┐
   │  CSV   │  │  JSON   │  │   PDF    │
   │Exporter│  │Exporter │  │ Exporter │
   └────┬───┘  └────┬────┘  └────┬─────┘
        │           │            │
        └───────────┴────────────┘
                    │
                    ▼
            ┌──────────────┐
            │  File Storage│
            └──────────────┘
```

## Features

### 1. Multiple Export Formats

- **CSV**: Tabular data format suitable for spreadsheets
- **JSON**: Complete structured data export with metadata
- **PDF**: Formatted report with pagination

### 2. Background Processing

- Export requests are processed asynchronously
- Jobs tracked with status (pending, processing, completed, failed)
- No timeout issues for large datasets

### 3. Date Range Filtering

- Export data within specific time ranges
- Maximum range: 1 year
- Prevents oversized exports

### 4. Automatic Cleanup

- Export files expire after 7 days
- Automatic deletion of expired files
- Prevents storage bloat

### 5. Progress Tracking

- Real-time status updates
- Record count and file size tracking
- Error reporting for failed exports

## API Endpoints

### Request Export

```http
POST /api/v1/export/request
Content-Type: application/json
Authorization: Bearer <token>

{
  "format": "csv|json|pdf",
  "start_date": "2024-01-01T00:00:00Z" (optional),
  "end_date": "2024-12-31T23:59:59Z" (optional)
}
```

**Response (202 Accepted)**:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "user_id": "...",
  "format": "csv",
  "status": "pending",
  "record_count": 0,
  "expires_at": "2024-12-08T...",
  "created_at": "2024-12-01T..."
}
```

### Check Export Status

```http
GET /api/v1/export/:id/status
Authorization: Bearer <token>
```

**Response (200 OK)**:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "user_id": "...",
  "format": "csv",
  "status": "completed",
  "file_path": "/exports/550e8400-e29b-41d4-a716-446655440000.csv",
  "file_size": 12345,
  "record_count": 150,
  "started_at": "2024-12-01T10:00:00Z",
  "completed_at": "2024-12-01T10:00:05Z",
  "expires_at": "2024-12-08T10:00:00Z",
  "created_at": "2024-12-01T10:00:00Z"
}
```

### Download Export

```http
GET /api/v1/export/:id/download
Authorization: Bearer <token>
```

**Response (200 OK)**:
- Content-Type: text/csv | application/json | application/pdf
- Content-Disposition: attachment; filename="20241201_donelist_export.csv"
- Binary file content

### Get Export History

```http
GET /api/v1/export/history?limit=10
Authorization: Bearer <token>
```

**Response (200 OK)**:
```json
{
  "jobs": [
    {
      "id": "...",
      "format": "csv",
      "status": "completed",
      "record_count": 150,
      "created_at": "2024-12-01T10:00:00Z"
    }
  ],
  "total": 5
}
```

## Export Status Flow

```
pending → processing → completed
              ↓
            failed
```

## Data Included in Exports

### Checkins
- ID
- Title (content)
- Description
- Category name
- Tags
- Priority
- Status
- Created/Updated timestamps

### Metadata
- Export timestamp
- Format
- Total records
- Filter applied flag
- Date range (if applicable)

## Export Format Examples

### CSV Format
```csv
ID,Title,Description,Category,Tags,Priority,Status,Created At,Updated At
550e8400...,Finished report,,Work,"urgent,important",high,completed,2024-11-01 10:00:00,2024-11-01 10:00:00
```

### JSON Format
```json
{
  "checkins": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "title": "Finished report",
      "description": null,
      "category": "Work",
      "tags": ["urgent", "important"],
      "priority": "high",
      "status": "completed",
      "created_at": "2024-11-01T10:00:00Z",
      "updated_at": "2024-11-01T10:00:00Z"
    }
  ],
  "metadata": {
    "exported_at": "2024-12-01T10:00:00Z",
    "export_format": "json",
    "total_records": 1,
    "filter_applied": false
  }
}
```

### PDF Format
- Professional formatted document
- Title and metadata header
- Paginated checkin list
- Category, tags, and status information
- Auto-pagination for large datasets

## Usage Example

```go
// Create export service
exportService := export.NewExportService(
    db,
    checkinRepo,
    categoryRepo,
    logger,
    "/var/donelist/exports",
)

// Request export
req := &export.ExportRequest{
    UserID: userID,
    Format: export.FormatCSV,
    StartDate: &startTime,
    EndDate: &endTime,
}

job, err := exportService.RequestExport(ctx, req)
if err != nil {
    // Handle error
}

// Check status later
job, err = exportService.GetExportJob(ctx, job.ID, userID)
if err != nil {
    // Handle error
}

if job.Status == export.StatusCompleted {
    // Download file
    filePath, err := exportService.GetExportFile(ctx, job.ID, userID)
    // Serve file to user
}
```

## Configuration

### Storage Directory

Set the export storage directory in the service initialization:

```go
storageDir := os.Getenv("EXPORT_STORAGE_DIR")
if storageDir == "" {
    storageDir = "./exports"
}
```

### Cleanup Job

Run periodic cleanup to remove expired exports:

```go
// Run daily cleanup
ticker := time.NewTicker(24 * time.Hour)
defer ticker.Stop()

for range ticker.C {
    err := exportService.CleanupExpiredExports(context.Background())
    if err != nil {
        logger.Error("Failed to cleanup exports", zap.Error(err))
    }
}
```

## Error Handling

### Common Errors

1. **Export already in progress**: Only one active export per user
2. **Invalid format**: Format must be csv, json, or pdf
3. **Invalid date range**: Start must be before end, max 1 year
4. **Export expired**: File deleted after 7 days
5. **Unauthorized access**: Users can only access their own exports

### Error Responses

```json
{
  "error": "you already have an export in progress"
}
```

## Security Considerations

1. **Authentication Required**: All endpoints require valid JWT token
2. **Ownership Verification**: Users can only access their own exports
3. **File Path Security**: Files stored with UUID-based names
4. **Automatic Cleanup**: Prevents indefinite storage of user data
5. **Rate Limiting**: Prevent export spam (one active export per user)

## Performance

### Optimization Strategies

1. **Background Processing**: No request timeout issues
2. **Pagination**: Handle large datasets efficiently
3. **Streaming**: Large files served directly from disk
4. **Indexing**: Fast lookups for export jobs by user and status
5. **Batch Processing**: Export up to 10,000 records per job

### Limits

- Max export range: 1 year
- Max records per export: 10,000
- File expiration: 7 days
- One active export per user

## Testing

Run tests:

```bash
go test -v ./internal/export/...
go test -v ./internal/api/handlers/... -run Export
```

## Future Enhancements

1. Email notification when export is ready
2. S3/cloud storage integration
3. Scheduled exports
4. Custom field selection
5. Excel format support
6. Export templates
7. Compression for large files
8. Progress percentage tracking
