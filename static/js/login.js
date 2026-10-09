// SPDX-License-Identifier: MIT
/* Sign-in form for the admin panel. Posts the credentials to /api/session;
   on success the server sets an HttpOnly session cookie and the admin page
   is loaded. Everything is inside one function so nothing leaks into globals. */
(function () {
  'use strict';

  var messages = {
    en: {
      username: 'Username', password: 'Password', signIn: 'Sign in',
      invalid: 'Wrong username or password.',
      locked: 'Too many failed attempts. Try again in a few minutes.',
      network: 'Could not reach the server. Check the connection and try again.',
      error: 'Could not sign in. Try again.'
    },
    ru: {
      username: 'Имя пользователя', password: 'Пароль', signIn: 'Войти',
      invalid: 'Неверное имя пользователя или пароль.',
      locked: 'Слишком много неудачных попыток. Попробуйте через несколько минут.',
      network: 'Нет связи с сервером. Проверьте подключение и попробуйте снова.',
      error: 'Не удалось войти. Попробуйте снова.'
    }
  };
  var lang = /^ru/i.test(navigator.language || '') ? 'ru' : 'en';
  var t = messages[lang];

  document.documentElement.lang = lang;
  document.querySelectorAll('[data-i18n]').forEach(function (el) {
    var key = el.getAttribute('data-i18n');
    if (t[key]) el.textContent = t[key];
  });

  var form = document.getElementById('loginForm');
  var errorBox = document.getElementById('loginError');
  var submit = document.getElementById('loginSubmit');

  function showError(text) {
    errorBox.textContent = text;
    errorBox.hidden = false;
  }

  form.addEventListener('submit', function (event) {
    event.preventDefault();
    errorBox.hidden = true;
    var username = form.elements.username.value;
    var password = form.elements.password.value;
    if (!username || !password) {
      showError(t.invalid);
      return;
    }
    submit.disabled = true;
    fetch('/api/session', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: username, password: password })
    }).then(function (res) {
      if (res.ok) {
        window.location.replace('/admin');
        return;
      }
      submit.disabled = false;
      form.elements.password.value = '';
      if (res.status === 401) showError(t.invalid);
      else if (res.status === 429) showError(t.locked);
      else showError(t.error);
    }).catch(function () {
      submit.disabled = false;
      showError(t.network);
    });
  });
})();
