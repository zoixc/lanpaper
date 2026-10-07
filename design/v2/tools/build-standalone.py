#!/usr/bin/env python3
"""Собирает одностраничную версию макета: design/v2/standalone.html.

Зачем: макет состоит из шести файлов и ссылается на шрифты и фото из
соседних каталогов. В обычном браузере это работает, а в просмотрщике,
который показывает один файл, — нет: подресурсы не подгружаются, страница
остаётся без стилей. Этот скрипт вклеивает CSS, JS, шрифты и демо-кадры
внутрь одного HTML.

Запуск:  python3 design/v2/tools/build-standalone.py
Требуется Pillow (кадры ужимаются до 900 px по длинной стороне, JPEG q80).
"""

import base64
import io
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
V2 = os.path.dirname(HERE)
REPO = os.path.dirname(os.path.dirname(V2))
MEDIA = os.path.join(REPO, "design", "prototypes", "media")
FONTS = os.path.join(REPO, "static", "fonts")
OUT = os.path.join(V2, "standalone.html")

MAX_SIDE = 900
JPEG_QUALITY = 80


def read(name, base=V2):
    with open(os.path.join(base, name), encoding="utf-8") as fh:
        return fh.read()


def data_uri(path, mime):
    with open(path, "rb") as fh:
        return "data:%s;base64,%s" % (mime, base64.b64encode(fh.read()).decode("ascii"))


def shrink(path):
    """Ужимает кадр и возвращает data-URI. PNG сохраняет прозрачность."""
    from PIL import Image

    img = Image.open(path)
    if max(img.size) > MAX_SIDE:
        scale = MAX_SIDE / max(img.size)
        img = img.resize((max(1, round(img.width * scale)), max(1, round(img.height * scale))),
                         Image.LANCZOS)
    buf = io.BytesIO()
    if os.path.splitext(path)[1].lower() == ".png":
        img.save(buf, format="PNG", optimize=True)
        mime = "image/png"
    else:
        img.convert("RGB").save(buf, format="JPEG", quality=JPEG_QUALITY, optimize=True, progressive=True)
        mime = "image/jpeg"
    return "data:%s;base64,%s" % (mime, base64.b64encode(buf.getvalue()).decode("ascii"))


def main():
    try:
        import PIL  # noqa: F401
    except ImportError:
        sys.exit("Нужен Pillow: python3 -m venv .venv && .venv/bin/pip install pillow")

    css = "\n".join(read(f) for f in ("tokens.css", "ui.css", "layout.css"))
    # Шрифты лежат в static/fonts и подключены относительным путём — вклеиваем.
    for name in re.findall(r"\.\./\.\./static/fonts/([\w.-]+\.woff2)", css):
        css = css.replace("../../static/fonts/" + name, data_uri(os.path.join(FONTS, name), "font/woff2"))

    js = "\n".join(read(f) for f in ("i18n.js", "data.js", "app.js"))
    # Демо-кадры: тот же путь, что в data.js, но с содержимым внутри файла.
    frames = {}
    for name in sorted(os.listdir(MEDIA)):
        frames[name] = shrink(os.path.join(MEDIA, name))
    # data.js склеивает путь как MEDIA + 'файл', а один и тот же кадр встречается
    # в данных трижды (кадр, превью, постер). Склеивать путь больше не нужно, а
    # каждый кадр должен лежать в файле ровно один раз — поэтому имя файла
    # заменяем ссылкой на карту, которую кладём в начало скрипта.
    js = re.sub(r"const MEDIA = '[^']*';", "const MEDIA = '';", js)
    names = sorted(frames)
    for name in names:
        js = js.replace("'" + name + "'", "window.LP_IMG['" + name + "']")
    prelude = "window.LP_IMG = {\n" + ",\n".join(
        "  %r: %r" % (name, frames[name]) for name in names) + "\n};\n"
    js = prelude + js

    html = read("index.html")
    html = re.sub(r'\s*<link rel="stylesheet"[^>]*>', "", html)
    html = re.sub(r'\s*<script src="(i18n|data|app)\.js"></script>', "", html)
    # Значок и иконка для домашнего экрана — тоже подресурсы: убираем, чтобы
    # в файле не осталось ни одного внешнего запроса. Шрифт уже внутри CSS.
    html = re.sub(r'\s*<link rel="(icon|apple-touch-icon|preload)"[^>]*>', "", html)
    html = html.replace("</head>", "<style>\n" + css + "\n</style>\n</head>", 1)
    html = html.replace("</body>", "<script>\n" + js + "\n</script>\n</body>", 1)
    # Знак в шапке — тот же favicon: вклеиваем, чтобы файл не тянул ничего извне.
    html = html.replace("../../static/favicon.svg", data_uri(os.path.join(REPO, "static", "favicon.svg"), "image/svg+xml"))
    html = html.replace("<title>", "<!-- Собрано tools/build-standalone.py: один файл, без внешних запросов -->\n<title>", 1)

    with open(OUT, "w", encoding="utf-8") as fh:
        fh.write(html)

    left = re.findall(r'(?:src|href)="(?!data:|#|https?:)([^"]+)"', html)
    print("Готово: %s — %.0f КБ, вклеено кадров: %d" % (OUT, os.path.getsize(OUT) / 1024, len(names)))
    print("Внешние ссылки в разметке:", ", ".join(sorted(set(left))) or "нет")


if __name__ == "__main__":
    main()
