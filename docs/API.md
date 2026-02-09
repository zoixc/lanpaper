# Lanpaper API Documentation

Comprehensive API reference for Lanpaper server.

## Table of Contents

- [Authentication](#authentication)
- [Endpoints](#endpoints)
  - [Health Check](#health-check)
  - [List Wallpapers](#list-wallpapers)
  - [Create Link](#create-link)
  - [Update Link](#update-link)
  - [Delete Link](#delete-link)
  - [Upload Image](#upload-image)
  - [List External Images](#list-external-images)
  - [Preview External Image](#preview-external-image)
- [Error Responses](#error-responses)
- [Rate Limiting](#rate-limiting)
- [Examples](#examples)

---

## Authentication

Lanpaper supports optional HTTP Basic Authentication. Authentication can be disabled by:

1. Setting `DISABLE_AUTH=true` environment variable
2. Not providing credentials (auto-disabled with warning)
3. Setting `"disableAuth": true` in `config.json`

When authentication is enabled, include credentials in requests:

```bash
curl -u admin:password https://lanpaper.example.com/api/wallpapers
```

Or use Authorization header:

```bash
curl -H "Authorization: Basic YWRtaW46cGFzc3dvcmQ=" https://lanpaper.example.com/api/wallpapers
```

---

## Endpoints

### Health Check

Check if the server is running.

**Endpoint:** `GET /health`

**Authentication:** Not required

**Response:**

```json
{
  "status": "ok",
  "service": "lanpaper",
  "time": 1707456347
}
```

**Example:**

```bash
curl https://lanpaper.example.com/health
```

---

### List Wallpapers

Retrieve all wallpaper links.

**Endpoint:** `GET /api/wallpapers`

**Authentication:** Required (if enabled)

**Response:**

```json
[
  {
    "id": "office-bg",
    "linkName": "office-bg",
    "imageUrl": "/static/images/office-bg.jpg",
    "preview": "/static/images/previews/office-bg.webp",
    "hasImage": true,
    "mimeType": "jpg",
    "sizeBytes": 245670,
    "modTime": 1707456000,
    "createdAt": 1707450000
  },
  {
    "id": "home-screen",
    "linkName": "home-screen",
    "imageUrl": "",
    "preview": "",
    "hasImage": false,
    "mimeType": "",
    "sizeBytes": 0,
    "modTime": 0,
    "createdAt": 1707455000
  }
]
```

**Example:**

```bash
curl -u admin:password https://lanpaper.example.com/api/wallpapers
```

---

### Create Link

Create a new wallpaper link (without image).

**Endpoint:** `POST /api/link`

**Authentication:** Required (if enabled)

**Request Body:**

```json
{
  "linkName": "my-wallpaper"
}
```

**Validation Rules:**
- Only alphanumeric characters, hyphens, and underscores
- Cannot be reserved names: `admin`, `api`, `static`, `health`
- Must be unique

**Response:** `201 Created`

```json
{
  "id": "my-wallpaper",
  "linkName": "my-wallpaper",
  "imageUrl": "",
  "preview": "",
  "hasImage": false,
  "mimeType": "",
  "sizeBytes": 0,
  "modTime": 0,
  "createdAt": 1707456500
}
```

**Examples:**

```bash
# Using curl
curl -X POST -u admin:password \
  -H "Content-Type: application/json" \
  -d '{"linkName":"office-wall"}' \
  https://lanpaper.example.com/api/link

# Using HTTPie
http POST https://lanpaper.example.com/api/link \
  linkName=office-wall \
  -a admin:password
```

**Error Responses:**

- `400 Bad Request` - Invalid link name
- `409 Conflict` - Link already exists

---

### Update Link

Update link properties (currently not implemented - reserved for future use).

**Endpoint:** `PUT /api/link/{linkName}`

**Authentication:** Required (if enabled)

---

### Delete Link

Delete a wallpaper link and its associated image.

**Endpoint:** `DELETE /api/link/{linkName}`

**Authentication:** Required (if enabled)

**Response:** `200 OK`

```json
{
  "message": "Link deleted"
}
```

**Example:**

```bash
curl -X DELETE -u admin:password \
  https://lanpaper.example.com/api/link/office-wall
```

**Error Responses:**

- `404 Not Found` - Link does not exist

---

### Upload Image

Upload an image to an existing link.

**Endpoint:** `POST /api/upload`

**Authentication:** Required (if enabled)

**Content-Type:** `multipart/form-data`

**Form Fields:**

| Field      | Type   | Required | Description                                    |
|------------|--------|----------|------------------------------------------------|
| linkName   | string | Yes      | The link ID to upload to                       |
| file       | file   | No*      | Image/video file                               |
| url        | string | No*      | URL to download image from or local file path  |

*Either `file` or `url` must be provided.

**Supported Formats:**

- **Images:** JPEG, PNG, GIF, WebP, BMP, TIFF
- **Videos:** MP4, WebM

**Size Limits:**

Configurable via `MAX_UPLOAD_MB` environment variable or `maxUploadMB` in config (default: 10 MB).

**Response:** `200 OK`

```json
{
  "id": "office-wall",
  "linkName": "office-wall",
  "imageUrl": "/static/images/office-wall.jpg",
  "preview": "/static/images/previews/office-wall.webp",
  "hasImage": true,
  "mimeType": "jpg",
  "sizeBytes": 245670,
  "modTime": 1707456800,
  "createdAt": 1707456500
}
```

**Examples:**

```bash
# Upload local file
curl -X POST -u admin:password \
  -F "linkName=office-wall" \
  -F "file=@/path/to/image.jpg" \
  https://lanpaper.example.com/api/upload

# Upload from URL
curl -X POST -u admin:password \
  -F "linkName=office-wall" \
  -F "url=https://example.com/image.jpg" \
  https://lanpaper.example.com/api/upload

# Upload from external directory (server-side)
curl -X POST -u admin:password \
  -F "linkName=office-wall" \
  -F "url=photos/beach.jpg" \
  https://lanpaper.example.com/api/upload
```

**Error Responses:**

- `400 Bad Request` - Invalid file, unsupported format, or file too large
- `404 Not Found` - Link does not exist
- `413 Payload Too Large` - File exceeds maximum size
- `429 Too Many Requests` - Rate limit exceeded or too many concurrent uploads

---

### List External Images

List available images from the external image directory.

**Endpoint:** `GET /api/external-images`

**Authentication:** Required (if enabled)

**Response:**

```json
{
  "images": [
    "photos/beach.jpg",
    "photos/mountains.png",
    "wallpapers/abstract.jpg"
  ]
}
```

**Example:**

```bash
curl -u admin:password https://lanpaper.example.com/api/external-images
```

**Configuration:**

Set external image directory:
- Environment: `EXTERNAL_IMAGE_DIR=/path/to/images`
- Config: `"externalImageDir": "/path/to/images"`
- Default: `external/images`

---

### Preview External Image

Generate a preview thumbnail for an external image.

**Endpoint:** `POST /api/external-image-preview`

**Authentication:** Required (if enabled)

**Request Body:**

```json
{
  "path": "photos/beach.jpg"
}
```

**Response:**

Returns WebP image data (binary) with `Content-Type: image/webp`.

**Example:**

```bash
curl -u admin:password \
  -H "Content-Type: application/json" \
  -d '{"path":"photos/beach.jpg"}' \
  https://lanpaper.example.com/api/external-image-preview \
  --output preview.webp
```

**Error Responses:**

- `400 Bad Request` - Invalid or unsafe path
- `404 Not Found` - Image not found
- `500 Internal Server Error` - Failed to generate preview

---

## Error Responses

All error responses follow this format:

```json
{
  "error": "Error message description"
}
```

### Common HTTP Status Codes

| Code | Meaning                 | Description                           |
|------|-------------------------|---------------------------------------|
| 400  | Bad Request             | Invalid input or malformed request    |
| 401  | Unauthorized            | Authentication required or failed     |
| 403  | Forbidden               | Access denied                         |
| 404  | Not Found               | Resource does not exist               |
| 409  | Conflict                | Resource already exists               |
| 413  | Payload Too Large       | File exceeds size limit               |
| 429  | Too Many Requests       | Rate limit exceeded                   |
| 500  | Internal Server Error   | Server-side error                     |

---

## Rate Limiting

Lanpaper implements rate limiting to prevent abuse.

**Default Limits:**

- Public endpoints: 50 requests/minute
- Admin endpoints: Unlimited (0 = no limit)
- Upload endpoint: 20 requests/minute
- Burst allowance: 10 requests

**Configuration:**

```json
{
  "rate": {
    "public_per_min": 50,
    "admin_per_min": 0,
    "upload_per_min": 20,
    "burst": 10
  }
}
```

Or via environment variable:

```bash
RATE_LIMIT=100  # Sets all limits to 100/min
```

**Rate Limit Headers:**

Responses include rate limit information:

```
X-RateLimit-Limit: 50
X-RateLimit-Remaining: 45
X-RateLimit-Reset: 1707456900
```

---

## Examples

### Complete Workflow

```bash
# 1. Check server health
curl https://lanpaper.example.com/health

# 2. Create a new link
curl -X POST -u admin:password \
  -H "Content-Type: application/json" \
  -d '{"linkName":"desktop-bg"}' \
  https://lanpaper.example.com/api/link

# 3. Upload an image
curl -X POST -u admin:password \
  -F "linkName=desktop-bg" \
  -F "file=@wallpaper.jpg" \
  https://lanpaper.example.com/api/upload

# 4. View the wallpaper
open https://lanpaper.example.com/desktop-bg

# 5. List all wallpapers
curl -u admin:password https://lanpaper.example.com/api/wallpapers

# 6. Delete the link
curl -X DELETE -u admin:password \
  https://lanpaper.example.com/api/link/desktop-bg
```

### Python Example

```python
import requests
from requests.auth import HTTPBasicAuth

BASE_URL = "https://lanpaper.example.com"
auth = HTTPBasicAuth("admin", "password")

# Create link
response = requests.post(
    f"{BASE_URL}/api/link",
    json={"linkName": "python-wall"},
    auth=auth
)
print(response.json())

# Upload image
with open("image.jpg", "rb") as f:
    response = requests.post(
        f"{BASE_URL}/api/upload",
        files={"file": f},
        data={"linkName": "python-wall"},
        auth=auth
    )
print(response.json())

# List wallpapers
response = requests.get(f"{BASE_URL}/api/wallpapers", auth=auth)
wallpapers = response.json()
for wp in wallpapers:
    print(f"{wp['id']}: {wp['imageUrl']}")
```

### JavaScript/Node.js Example

```javascript
const axios = require('axios');
const FormData = require('form-data');
const fs = require('fs');

const BASE_URL = 'https://lanpaper.example.com';
const auth = {
  username: 'admin',
  password: 'password'
};

// Create link
async function createLink() {
  const response = await axios.post(
    `${BASE_URL}/api/link`,
    { linkName: 'js-wall' },
    { auth }
  );
  console.log(response.data);
}

// Upload image
async function uploadImage() {
  const form = new FormData();
  form.append('linkName', 'js-wall');
  form.append('file', fs.createReadStream('image.jpg'));
  
  const response = await axios.post(
    `${BASE_URL}/api/upload`,
    form,
    {
      auth,
      headers: form.getHeaders()
    }
  );
  console.log(response.data);
}

// List wallpapers
async function listWallpapers() {
  const response = await axios.get(
    `${BASE_URL}/api/wallpapers`,
    { auth }
  );
  response.data.forEach(wp => {
    console.log(`${wp.id}: ${wp.imageUrl}`);
  });
}
```

### Shell Script Example

```bash
#!/bin/bash

BASE_URL="https://lanpaper.example.com"
USER="admin"
PASS="password"

# Function to create link and upload
upload_wallpaper() {
    local link_name="$1"
    local file_path="$2"
    
    echo "Creating link: $link_name"
    curl -s -X POST -u "$USER:$PASS" \
        -H "Content-Type: application/json" \
        -d "{\"linkName\":\"$link_name\"}" \
        "$BASE_URL/api/link"
    
    echo -e "\nUploading image..."
    curl -s -X POST -u "$USER:$PASS" \
        -F "linkName=$link_name" \
        -F "file=@$file_path" \
        "$BASE_URL/api/upload"
    
    echo -e "\nDone! View at: $BASE_URL/$link_name"
}

# Usage
upload_wallpaper "my-desktop" "/path/to/wallpaper.jpg"
```

---

## Security Considerations

### File Upload Security

1. **MIME Type Validation**: Server validates file type using magic bytes
2. **Size Limits**: Configurable upload size limits
3. **Path Traversal Protection**: Strict path validation for external files
4. **Content Sanitization**: Filenames are sanitized to prevent injection

### Best Practices

1. **Use HTTPS**: Always use TLS/SSL in production
2. **Strong Passwords**: Use passwords with 16+ characters
3. **Rate Limiting**: Keep rate limits enabled
4. **Regular Updates**: Keep server and dependencies updated
5. **Firewall**: Restrict access to trusted networks if possible
6. **Monitoring**: Monitor logs for suspicious activity

### Configuration Security

```bash
# Recommended production settings
ADMIN_USER=admin
ADMIN_PASS=<strong-password-here>
MAX_UPLOAD_MB=50
RATE_LIMIT=100
INSECURE_SKIP_VERIFY=false
DISABLE_AUTH=false
```

---

## Additional Resources

- [Main README](../README.md)
- [Configuration Guide](CONFIGURATION.md)
- [Deployment Guide](DEPLOYMENT.md)
- [GitHub Repository](https://github.com/zoixc/lanpaper)
