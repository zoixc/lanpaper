# Lanpaper Feature Roadmap

Детальный план реализации новых функций с приоритетами и техническими спецификациями.

---

## 🎯 Priority 1: Core UX Improvements

### 1.1 Закрепление ссылок (Pin/Favorite)

**Описание:** Пользователь может отметить важные ссылки звёздочкой. Закрепленные ссылки отображаются в начале списка независимо от сортировки.

**Backend изменения:**

```go
// storage/wallpaper.go
type Wallpaper struct {
    // ... existing fields
    IsPinned  bool  `json:"isPinned"`
    PinnedAt  int64 `json:"pinnedAt,omitempty"` // timestamp when pinned
}

// Обновить sortSnap для учета pinned:
func sortSnap(snap []*Wallpaper) {
    sort.Slice(snap, func(i, j int) bool {
        // Pinned always first
        if snap[i].IsPinned != snap[j].IsPinned {
            return snap[i].IsPinned
        }
        // Then by image presence
        if snap[i].HasImage != snap[j].HasImage {
            return snap[i].HasImage
        }
        // Then by modification time
        if snap[i].HasImage {
            return snap[i].ModTime > snap[j].ModTime
        }
        return snap[i].CreatedAt > snap[j].CreatedAt
    })
}
```

**API endpoints:**

```go
// handlers/admin.go
// PATCH /api/link/{linkName}/pin
func HandleTogglePin(w http.ResponseWriter, r *http.Request) {
    // Toggle isPinned field
    // Set/unset pinnedAt timestamp
    // Return updated wallpaper
}
```

**Frontend изменения:**

```javascript
// static/js/app.js
// Add pin button to card
const pinBtn = card.querySelector('.pin-btn');
pinBtn.addEventListener('click', async () => {
    const isPinned = !link.isPinned;
    try {
        const updated = await apiCall(
            `/api/link/${encodeURIComponent(link.linkName)}/pin`,
            'PATCH',
            { isPinned }
        );
        link.isPinned = updated.isPinned;
        updatePinIcon(pinBtn, updated.isPinned);
        filterAndSort(); // Re-render with new order
        showToast(
            isPinned ? t('pinned', 'Pinned') : t('unpinned', 'Unpinned'),
            'success'
        );
    } catch (e) {}
});
```

**CSS:**

```css
.pin-btn {
    opacity: 0.3;
    transition: opacity 0.2s ease, transform 0.2s ease;
}
.pin-btn:hover { opacity: 0.7; }
.pin-btn.pinned {
    opacity: 1;
    color: var(--accent);
}
.pin-btn.pinned svg {
    fill: currentColor;
}
```

**i18n ключи:**
```json
{
    "pin": "Pin",
    "unpin": "Unpin",
    "pinned": "Pinned to top",
    "unpinned": "Unpinned",
    "aria_pin": "Pin this link",
    "aria_unpin": "Unpin this link"
}
```

**Тестирование:**
- [ ] Pin/unpin работает корректно
- [ ] Pinned ссылки всегда вверху
- [ ] Сортировка не влияет на pinned
- [ ] Состояние сохраняется после перезагрузки
- [ ] Работает в grid и list view

---

### 1.2 Плейлисты и расписание (Playlist/Schedule)

**Описание:** Ссылка может показывать несколько изображений по расписанию или циклически с интервалом.

**Концепция:**
- **Режим 1: Циклическое слайдшоу** — каждые N секунд/минут переключается на следующее изображение
- **Режим 2: Расписание** — в определенное время дня показывается конкретное изображение
- **Режим 3: Дни недели** — разные изображения для разных дней

**Backend изменения:**

```go
// storage/wallpaper.go
type ScheduleRule struct {
    Type      string `json:"type"`      // "time", "weekday", "interval"
    ImagePath string `json:"imagePath"` // relative path to image
    
    // For "time" type
    StartTime string `json:"startTime,omitempty"` // "09:00"
    EndTime   string `json:"endTime,omitempty"`   // "17:00"
    
    // For "weekday" type
    Weekdays []int `json:"weekdays,omitempty"` // [1,2,3,4,5] = Mon-Fri
    
    // For "interval" type
    IntervalSeconds int `json:"intervalSeconds,omitempty"` // 300 = 5 min
    Order           int `json:"order,omitempty"`           // sequence position
}

type Wallpaper struct {
    // ... existing fields
    PlaylistMode bool            `json:"playlistMode"`
    Schedule     []ScheduleRule  `json:"schedule,omitempty"`
    PlaylistImages []string      `json:"playlistImages,omitempty"` // array of image paths
}
```

**Логика выбора изображения:**

```go
// handlers/public.go
func SelectImageBySchedule(wp *Wallpaper) string {
    if !wp.PlaylistMode || len(wp.Schedule) == 0 {
        return wp.ImageURL // fallback to single image
    }
    
    now := time.Now()
    
    for _, rule := range wp.Schedule {
        switch rule.Type {
        case "time":
            if matchesTimeRange(now, rule.StartTime, rule.EndTime) {
                return rule.ImagePath
            }
        case "weekday":
            if containsInt(rule.Weekdays, int(now.Weekday())) {
                return rule.ImagePath
            }
        case "interval":
            // Cycle through playlistImages based on time
            idx := (int(now.Unix()) / rule.IntervalSeconds) % len(wp.PlaylistImages)
            return wp.PlaylistImages[idx]
        }
    }
    
    return wp.ImageURL // default fallback
}
```

**API endpoints:**

```go
// POST /api/link/{linkName}/schedule
func HandleAddScheduleRule(w http.ResponseWriter, r *http.Request) {
    // Add new schedule rule
    // Validate time ranges, weekdays, intervals
    // Save and return updated wallpaper
}

// DELETE /api/link/{linkName}/schedule/{index}
func HandleRemoveScheduleRule(w http.ResponseWriter, r *http.Request) {
    // Remove rule at index
}

// PUT /api/link/{linkName}/playlist
func HandleUpdatePlaylist(w http.ResponseWriter, r *http.Request) {
    // Update playlistImages array
    // Enable/disable playlistMode
}
```

**Frontend UI:**

```
┌─────────────────────────────────────┐
│ 📷 bedroom                          │
│                                     │
│ ⚙️ Playlist Settings                │
│                                     │
│ ○ Single image (current)            │
│ ● Playlist / Schedule               │
│                                     │
│ Mode:                               │
│ ☑ Interval rotation (5 min)        │
│ ☐ Time-based schedule               │
│ ☐ Day of week schedule              │
│                                     │
│ Images in playlist (3):             │
│ [thumbnail1] [thumbnail2] [thumb3]  │
│ + Add image                         │
│                                     │
│ [Save] [Cancel]                     │
└─────────────────────────────────────┘
```

**Тестирование:**
- [ ] Interval rotation работает корректно
- [ ] Time-based schedule активируется в нужное время
- [ ] Weekday schedule учитывает день недели
- [ ] Fallback на default image если правила не совпали
- [ ] Переключение между режимами сохраняется

---

## 🎨 Priority 2: Format Support & Processing

### 2.1 Поддержка SVG

**Backend:**
```go
// utils/validation.go
var allowedTypes = map[string]string{
    // ... existing types
    "image/svg+xml": "svg",
}

// Validate SVG content for security
func ValidateSVG(data []byte) error {
    // Check for dangerous tags: <script>, <object>, <embed>, <iframe>
    // Parse as XML and sanitize
    // Return error if contains potentially harmful content
}
```

**Security considerations:**
- Sanitize SVG: удалять `<script>`, event handlers (`onclick` etc.)
- Use Content-Security-Policy для SVG
- Рассмотреть конвертацию SVG → PNG для preview

---

### 2.2 GIF оптимизация

**Подход:**
- Use `github.com/disintegration/imaging` для обработки
- Уменьшить количество цветов (dithering)
- Resize если слишком большой
- Опционально: конвертация в WebP animated

```go
// utils/compression.go
func OptimizeGIF(input []byte, maxWidth, maxHeight int) ([]byte, error) {
    // Decode GIF
    // For each frame:
    //   - Resize if needed
    //   - Reduce color palette
    // Re-encode with optimized settings
}
```

---

### 2.3 Автоматическая конвертация форматов

**HEIC → JPEG:**
```go
import "github.com/jdeng/goheif"

func ConvertHEIC(input []byte) ([]byte, error) {
    // Decode HEIC
    // Convert to JPEG with quality 85
    // Return JPEG bytes
}
```

**AVIF → WebP:**
```go
import "github.com/Kagami/go-avif"

func ConvertAVIF(input []byte) ([]byte, error) {
    // Decode AVIF
    // Encode as WebP
}
```

**Frontend detection:**
```javascript
// Detect file format before upload
const ext = file.name.split('.').pop().toLowerCase();
if (['heic', 'heif', 'avif'].includes(ext)) {
    showToast(t('converting_format', 'Converting format...'), 'info');
}
```

---

### 2.4 Drag & Drop с автосозданием ссылки

**Концепция:** Перетащить файл в приложение → автоматически создать ссылку с именем файла (без расширения) → загрузить изображение.

**Frontend:**

```javascript
// static/js/app.js
function setupGlobalDropZone() {
    document.body.addEventListener('dragover', (e) => {
        e.preventDefault();
        if (!e.target.closest('.link-card')) {
            // Show drop overlay
            showDropOverlay();
        }
    });
    
    document.body.addEventListener('drop', async (e) => {
        e.preventDefault();
        hideDropOverlay();
        
        if (e.target.closest('.link-card')) return; // Already handled
        
        const files = Array.from(e.dataTransfer.files);
        for (const file of files) {
            const linkName = sanitizeLinkName(file.name);
            
            try {
                // Create link
                await apiCall('/api/link', 'POST', { linkName });
                
                // Upload file
                const formData = new FormData();
                formData.append('linkName', linkName);
                formData.append('file', file);
                await apiCall('/api/upload', 'POST', formData, true);
                
                showToast(
                    t('link_created_uploaded', 'Created "{name}" and uploaded')
                        .replace('{name}', linkName),
                    'success'
                );
            } catch (e) {
                // Handle errors
            }
        }
        
        await loadLinks();
    });
}

function sanitizeLinkName(filename) {
    // Remove extension
    let name = filename.replace(/\.[^.]+$/, '');
    // Replace spaces and special chars
    name = name.replace(/[^a-zA-Z0-9]+/g, '-');
    // Remove leading/trailing hyphens
    name = name.replace(/^-+|-+$/g, '');
    // Lowercase
    return name.toLowerCase();
}
```

**UI визуальная обратная связь:**
```css
.drop-overlay {
    position: fixed;
    inset: 0;
    background: rgba(var(--accent-rgb), 0.1);
    backdrop-filter: blur(4px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 9998;
    pointer-events: none;
}

.drop-overlay-content {
    padding: 40px;
    border: 3px dashed var(--accent);
    border-radius: var(--radius-lg);
    background: var(--card-bg);
    text-align: center;
}
```

---

## 📊 Priority 3: Analytics & Monitoring

### 3.1 Логи доступа

**Backend:**

```go
// storage/analytics.go
type AccessLog struct {
    LinkName  string    `json:"linkName"`
    Timestamp time.Time `json:"timestamp"`
    IP        string    `json:"ip"`
    UserAgent string    `json:"userAgent"`
    Referer   string    `json:"referer,omitempty"`
}

var accessLogs = []AccessLog{}
var logsMutex sync.RWMutex

func LogAccess(linkName, ip, ua, referer string) {
    logsMutex.Lock()
    defer logsMutex.Unlock()
    
    accessLogs = append(accessLogs, AccessLog{
        LinkName:  linkName,
        Timestamp: time.Now(),
        IP:        ip,
        UserAgent: ua,
        Referer:   referer,
    })
    
    // Keep only last 10000 entries in memory
    if len(accessLogs) > 10000 {
        accessLogs = accessLogs[len(accessLogs)-10000:]
    }
}

// Persist to file daily
func SaveAccessLogs() error {
    filename := fmt.Sprintf("data/logs/access-%s.json", time.Now().Format("2006-01-02"))
    // Write accessLogs to file
}
```

**Middleware:**
```go
// middleware/logging.go
func AccessLogging(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Skip admin/api paths
        if strings.HasPrefix(r.URL.Path, "/admin") || 
           strings.HasPrefix(r.URL.Path, "/api") {
            next.ServeHTTP(w, r)
            return
        }
        
        linkName := strings.TrimPrefix(r.URL.Path, "/")
        if linkName != "" {
            ip := r.RemoteAddr
            if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
                ip = xff
            }
            
            storage.LogAccess(
                linkName,
                ip,
                r.Header.Get("User-Agent"),
                r.Header.Get("Referer"),
            )
        }
        
        next.ServeHTTP(w, r)
    })
}
```

**API endpoints:**
```go
// GET /api/analytics/logs?linkName=bedroom&days=7
func HandleGetLogs(w http.ResponseWriter, r *http.Request) {
    // Return access logs filtered by linkName and time range
}

// GET /api/analytics/summary
func HandleGetSummary(w http.ResponseWriter, r *http.Request) {
    // Return aggregated stats: views per link, top referers, etc.
}
```

---

### 3.2 Статистика и графики

**Метрики для отображения:**
- Общее количество просмотров за период
- Просмотры по каждой ссылке
- Топ-5 самых популярных ссылок
- График просмотров по дням/часам
- География (если IP → GeoIP)
- User Agents (Desktop/Mobile/Other)

**Frontend visualization:**

Использовать легковесную библиотеку для графиков:
- Chart.js (simple, хорошо документирована)
- ApexCharts (более функциональная)
- Или native Canvas API для простых графиков

```html
<!-- admin.html - новая секция Analytics -->
<section class="section analytics-section">
    <h2 data-i18n="analytics">Analytics</h2>
    
    <div class="stats-grid">
        <div class="stat-card">
            <div class="stat-value" id="totalViews">0</div>
            <div class="stat-label" data-i18n="total_views">Total Views</div>
        </div>
        <div class="stat-card">
            <div class="stat-value" id="todayViews">0</div>
            <div class="stat-label" data-i18n="today_views">Today</div>
        </div>
        <div class="stat-card">
            <div class="stat-value" id="weekViews">0</div>
            <div class="stat-label" data-i18n="week_views">This Week</div>
        </div>
    </div>
    
    <div class="chart-container">
        <canvas id="viewsChart"></canvas>
    </div>
    
    <div class="top-links">
        <h3 data-i18n="top_links">Most Viewed Links</h3>
        <ul id="topLinksList"></ul>
    </div>
</section>
```

**CSS для аналитики:**
```css
.stats-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 16px;
    margin-bottom: 24px;
}

.stat-card {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 20px;
    text-align: center;
}

.stat-value {
    font-family: var(--font-mono-bold);
    font-size: 2rem;
    color: var(--accent);
    margin-bottom: 8px;
}

.stat-label {
    font-size: var(--text-sm);
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
}

.chart-container {
    background: var(--card-bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 20px;
    margin-bottom: 24px;
}
```

---

## 🔐 Priority 4: Security & Access Control

### 4.1 Приватные ссылки с токенами

**Концепция:** Некоторые ссылки могут требовать токен в URL или header для доступа.

**Backend изменения:**

```go
// storage/wallpaper.go
type Wallpaper struct {
    // ... existing fields
    IsPrivate   bool   `json:"isPrivate"`
    AccessToken string `json:"accessToken,omitempty"`
    ExpiresAt   int64  `json:"expiresAt,omitempty"` // Unix timestamp, 0 = never
}

// Generate secure random token
func GenerateAccessToken() string {
    b := make([]byte, 32)
    rand.Read(b)
    return base64.URLEncoding.EncodeToString(b)
}
```

**Middleware для проверки:**
```go
// middleware/access_control.go
func RequireToken(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        linkName := extractLinkName(r.URL.Path)
        
        wp, ok := storage.Global.Get(linkName)
        if !ok || !wp.IsPrivate {
            next.ServeHTTP(w, r)
            return
        }
        
        // Check expiration
        if wp.ExpiresAt > 0 && time.Now().Unix() > wp.ExpiresAt {
            http.Error(w, "Link expired", http.StatusGone)
            return
        }
        
        // Check token in query or header
        token := r.URL.Query().Get("token")
        if token == "" {
            token = r.Header.Get("X-Access-Token")
        }
        
        if token != wp.AccessToken {
            http.Error(w, "Forbidden", http.StatusForbidden)
            return
        }
        
        next.ServeHTTP(w, r)
    })
}
```

**API endpoints:**
```go
// POST /api/link/{linkName}/privacy
func HandleUpdatePrivacy(w http.ResponseWriter, r *http.Request) {
    var req struct {
        IsPrivate bool   `json:"isPrivate"`
        ExpiresIn int64  `json:"expiresIn"` // seconds from now, 0 = never
    }
    
    // Parse request
    // Generate new token if enabling privacy
    // Set expiration timestamp
    // Save and return updated wallpaper with token
}

// POST /api/link/{linkName}/regenerate-token
func HandleRegenerateToken(w http.ResponseWriter, r *http.Request) {
    // Generate new access token
    // Invalidate old token
}
```

**Frontend UI:**

```html
<!-- Privacy settings modal -->
<div class="privacy-settings">
    <label>
        <input type="checkbox" id="isPrivateCheckbox">
        <span data-i18n="make_private">Make this link private</span>
    </label>
    
    <div id="privacyOptions" class="hidden">
        <label>
            <span data-i18n="expiration">Expiration:</span>
            <select id="expirationSelect">
                <option value="0" data-i18n="never_expires">Never</option>
                <option value="3600">1 hour</option>
                <option value="86400">1 day</option>
                <option value="604800">1 week</option>
                <option value="2592000">30 days</option>
            </select>
        </label>
        
        <div class="token-display">
            <label data-i18n="access_token">Access Token:</label>
            <div class="token-value">
                <code id="tokenValue">...</code>
                <button class="btn btn--secondary" id="copyTokenBtn">
                    <span data-i18n="copy">Copy</span>
                </button>
            </div>
            <button class="btn btn--secondary" id="regenerateTokenBtn">
                <span data-i18n="regenerate_token">Regenerate Token</span>
            </button>
        </div>
        
        <div class="url-examples">
            <p data-i18n="usage_example">Usage example:</p>
            <code>https://yourserver.com/bedroom?token=ABC123...</code>
        </div>
    </div>
</div>
```

---

## 📋 Implementation Checklist

### Phase 1: Core UX (2-3 days)
- [ ] Backend: Add `isPinned`, `pinnedAt` fields
- [ ] Backend: Update sorting logic
- [ ] API: `PATCH /api/link/{linkName}/pin`
- [ ] Frontend: Pin button UI
- [ ] Frontend: Pin state management
- [ ] i18n: Add translation keys
- [ ] Testing: Pin/unpin functionality

### Phase 2: Playlists Basic (3-4 days)
- [ ] Backend: Add `playlistMode`, `schedule`, `playlistImages` fields
- [ ] Backend: Implement `SelectImageBySchedule` logic
- [ ] API: Schedule management endpoints
- [ ] Frontend: Playlist settings modal
- [ ] Frontend: Image selection for playlist
- [ ] Testing: Interval rotation

### Phase 3: Playlists Advanced (2-3 days)
- [ ] Backend: Time-based schedule logic
- [ ] Backend: Weekday schedule logic
- [ ] Frontend: Schedule rule builder UI
- [ ] Frontend: Visual schedule preview
- [ ] Testing: Complex schedule scenarios

### Phase 4: Format Support (3-4 days)
- [ ] Backend: SVG support + sanitization
- [ ] Backend: GIF optimization
- [ ] Backend: HEIC/AVIF conversion
- [ ] Frontend: Format detection
- [ ] Frontend: Conversion progress indicator
- [ ] Testing: All format conversions

### Phase 5: Drag & Drop (1-2 days)
- [ ] Frontend: Global drop zone
- [ ] Frontend: Auto link name sanitization
- [ ] Frontend: Batch upload support
- [ ] Frontend: Visual feedback
- [ ] Testing: Multi-file drop

### Phase 6: Analytics Backend (2-3 days)
- [ ] Backend: Access log storage
- [ ] Backend: Log rotation & persistence
- [ ] Middleware: Logging middleware
- [ ] API: Analytics endpoints
- [ ] Testing: Log collection

### Phase 7: Analytics Frontend (2-3 days)
- [ ] Frontend: Stats dashboard
- [ ] Frontend: Charts integration
- [ ] Frontend: Date range selector
- [ ] Frontend: Export logs feature
- [ ] Testing: Data visualization

### Phase 8: Private Links (2-3 days)
- [ ] Backend: Token generation
- [ ] Backend: Access control middleware
- [ ] Backend: Expiration logic
- [ ] API: Privacy management endpoints
- [ ] Frontend: Privacy settings UI
- [ ] Frontend: Token management
- [ ] Testing: Token validation

---

## 🗓️ Estimated Timeline

**Total development time:** ~20-25 days

- **Week 1:** Pin/favorite + Playlist basic
- **Week 2:** Playlist advanced + Format support
- **Week 3:** Drag & Drop + Analytics backend
- **Week 4:** Analytics frontend + Private links

---

## 🔧 Technical Dependencies

### New Go packages:
```bash
go get github.com/jdeng/goheif           # HEIC support
go get github.com/Kagami/go-avif         # AVIF support
go get github.com/disintegration/imaging # GIF optimization
```

### Frontend libraries (optional):
- Chart.js or ApexCharts для графиков
- date-fns для работы с датами в schedule UI

---

## 📝 Notes

- Все изменения обратно совместимы — старые данные будут работать
- Новые поля опциональны и имеют значения по умолчанию
- API endpoints следуют RESTful конвенциям
- i18n ключи готовы для всех языков
- Все функции работают в темной и светлой теме
