# View Toggle Morphing Animation

## 🎯 Что это?

Полностью CSS/SVG анимация переключателя вида с морфинг-эффектом:

- **Список** (3 полоски) ⟷ **Сетка** (6 квадратиков)
- Плавная трансформация без смены картинок
- Адаптируется под светлую/тёмную тему
- Легковесно (~4KB CSS)

---

## 📦 Файлы

1. **`static/css/view-toggle-morphing.css`** — стили и анимация
2. **`static/js/view-toggle-morphing.js`** — логика переключения
3. **`static/view-toggle-morphing.html`** — HTML структура кнопки

---

## 🔧 Интеграция

### Шаг 1: Подключить CSS и JS в `admin.html`

Добавь в `<head>`:

```html
<link rel="stylesheet" href="/static/css/view-toggle-morphing.css">
```

Добавь перед закрывающим `</body>`:

```html
<script src="/static/js/view-toggle-morphing.js" defer></script>
```

### Шаг 2: Заменить кнопку в HTML

**Найди старый код** (в блоке `.controls`):

```html
<button id="viewToggle" class="theme-switcher" aria-label="Toggle view">
  <img src="/static/icons/grid-dark.png" class="view-icon list-icon light-theme-icon" alt="List">
  <img src="/static/icons/grid-light.png" class="view-icon list-icon dark-theme-icon" alt="List">
  <img src="/static/icons/list-dark.png" class="view-icon grid-icon light-theme-icon active" alt="Grid">
  <img src="/static/icons/list-light.png" class="view-icon grid-icon dark-theme-icon active" alt="Grid">
</button>
```

**Замени на новый код**:

```html
<button id="viewToggleMorphing" class="view-toggle-morphing" data-view="list" aria-label="Switch to grid view">
    <svg class="view-icon-svg" viewBox="0 0 18 18" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <!-- Main bars (left column in grid, full width in list) -->
        <rect class="view-bar view-bar-1" />
        <rect class="view-bar view-bar-2" />
        <rect class="view-bar view-bar-3" />
        
        <!-- Clone bars (right column in grid, hidden in list) -->
        <rect class="view-bar-1-clone" />
        <rect class="view-bar-2-clone" />
        <rect class="view-bar-3-clone" />
    </svg>
</button>
```

### Шаг 3: Удалить старый JS (опционально)

Если в `app.js` есть старая логика для `#viewToggle`, можно удалить или закомментировать — новый JS работает независимо.

---

## 🎨 Как это работает?

### CSS Magic

```css
/* Список: 3 широкие полоски */
[data-view="list"] .view-bar-1 {
    x: 1; y: 2; width: 16; height: 2.5;
}

/* Сетка: узкие квадраты + клоны справа */
[data-view="grid"] .view-bar-1 {
    x: 1; y: 1; width: 6.5; height: 4.5;
}

[data-view="grid"] .view-bar-1-clone {
    x: 10.5; y: 1; width: 6.5; height: 4.5;
    opacity: 1; /* появляется */
}
```

### JavaScript

```javascript
// Переключение атрибута data-view
viewToggle.setAttribute('data-view', newView);

// CSS автоматически анимирует трансформацию
```

---

## 🚀 Преимущества

✅ **Плавная анимация** — морфинг вместо резкой смены PNG  
✅ **Легковесно** — 4KB CSS vs 4 PNG файла  
✅ **Адаптивно** — цвет меняется с темой автоматически  
✅ **Доступно** — ARIA-labels + semantic HTML  
✅ **Современно** — чистый CSS без библиотек

---

## 🎯 Демо

**Список → Сетка:**
```
▬▬▬▬▬▬▬  →  ▪▪  ▪▪
▬▬▬▬▬▬▬  →  ▪▪  ▪▪
▬▬▬▬▬▬▬  →  ▪▪  ▪▪
```

Каждая полоска **разделяется** на 2 квадрата с плавной анимацией!

---

## ⚙️ Настройка

### Изменить скорость анимации

В `view-toggle-morphing.css`:

```css
.view-bar {
    transition: all 0.35s cubic-bezier(0.4, 0, 0.2, 1);
    /*              ^^^^^ измени здесь */
}
```

### Изменить цвет

Цвет наследуется от `color: var(--text)`. Если нужен кастомный:

```css
.view-toggle-morphing {
    color: #your-color;
}
```

---

## 🐛 Поддержка браузеров

✅ Chrome 90+  
✅ Firefox 88+  
✅ Safari 14+  
✅ Edge 90+

*(CSS transitions + SVG attributes)*

---

## 📝 Лицензия

Массивный респект за крутую идею! 🎨  
Используй как хочешь, без ограничений.
