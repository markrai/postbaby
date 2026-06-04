package server

import "html/template"

var authPageTemplate = template.Must(template.New("auth-page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{ .Title }}</title>
  <style>
    :root {
      color-scheme: light;
      font-family: Georgia, "Times New Roman", serif;
      background: #f4efe5;
      color: #1f1a14;
    }
    body {
      margin: 0;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      background:
        radial-gradient(circle at top left, rgba(213, 185, 141, 0.28), transparent 40%),
        linear-gradient(180deg, #f8f4ec 0%, #ece3d4 100%);
    }
    .card {
      width: min(420px, calc(100vw - 32px));
      padding: 28px;
      border-radius: 18px;
      background: rgba(255, 251, 244, 0.94);
      border: 1px solid rgba(115, 91, 57, 0.18);
      box-shadow: 0 18px 50px rgba(59, 42, 20, 0.12);
    }
    h1 {
      margin: 0 0 10px;
      font-size: 30px;
      line-height: 1.1;
    }
    p {
      margin: 0 0 18px;
      line-height: 1.5;
      color: #574a3a;
    }
    label {
      display: block;
      margin-bottom: 14px;
      font-size: 14px;
      font-weight: 600;
    }
    input {
      width: 100%;
      margin-top: 6px;
      padding: 12px 14px;
      border-radius: 12px;
      border: 1px solid #cdbda6;
      background: #fffdfa;
      box-sizing: border-box;
      font: inherit;
    }
    input:focus {
      outline: 2px solid #b86f37;
      outline-offset: 2px;
    }
    .error {
      margin: 0 0 18px;
      padding: 12px 14px;
      border-radius: 12px;
      background: #fdeceb;
      color: #8d2f28;
      border: 1px solid #f3c2bf;
      font-size: 14px;
    }
    .hint {
      font-size: 13px;
      color: #6b5c49;
      margin-bottom: 18px;
    }
    button {
      width: 100%;
      padding: 12px 16px;
      border: 0;
      border-radius: 999px;
      background: #2f2418;
      color: #fff8ed;
      font: inherit;
      cursor: pointer;
    }
    button:hover {
      background: #433224;
    }
  </style>
</head>
<body>
  <main class="card">
    <h1>{{ .Heading }}</h1>
    <p>{{ .Body }}</p>
    {{ if .Error }}<div class="error">{{ .Error }}</div>{{ end }}
    <form method="post" action="{{ .Action }}">
      <label>
        Username
        <input type="text" name="username" autocomplete="username" value="{{ .Username }}" required>
      </label>
      <label>
        Password
        <input type="password" name="password" autocomplete="{{ if .ShowConfirm }}new-password{{ else }}current-password{{ end }}" required>
      </label>
      {{ if .ShowConfirm }}
      <label>
        Confirm password
        <input type="password" name="confirmPassword" autocomplete="new-password" required>
      </label>
      <div class="hint">Use at least {{ .PasswordMinLength }} characters.</div>
      {{ end }}
      <button type="submit">{{ .SubmitLabel }}</button>
    </form>
  </main>
</body>
</html>
`))

var manageAccountsPageTemplate = template.Must(template.New("manage-accounts-page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{ .Title }}</title>
  <style>
    :root {
      color-scheme: light;
      font-family: Georgia, "Times New Roman", serif;
      background: #f4efe5;
      color: #1f1a14;
    }
    body {
      margin: 0;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      background:
        radial-gradient(circle at top left, rgba(213, 185, 141, 0.28), transparent 40%),
        linear-gradient(180deg, #f8f4ec 0%, #ece3d4 100%);
      padding: 16px;
      box-sizing: border-box;
    }
    .card {
      width: min(720px, calc(100vw - 32px));
      padding: 28px;
      border-radius: 18px;
      background: rgba(255, 251, 244, 0.94);
      border: 1px solid rgba(115, 91, 57, 0.18);
      box-shadow: 0 18px 50px rgba(59, 42, 20, 0.12);
    }
    h1 {
      margin: 0 0 10px;
      font-size: 30px;
      line-height: 1.1;
    }
    h2 {
      margin: 26px 0 12px;
      font-size: 20px;
      line-height: 1.2;
    }
    p {
      margin: 0 0 18px;
      line-height: 1.5;
      color: #574a3a;
    }
    .error {
      margin: 0 0 18px;
      padding: 12px 14px;
      border-radius: 12px;
      background: #fdeceb;
      color: #8d2f28;
      border: 1px solid #f3c2bf;
      font-size: 14px;
    }
    .accounts-list {
      margin: 0;
      padding: 0;
      list-style: none;
      display: grid;
      gap: 10px;
    }
    .accounts-list li {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      align-items: center;
      padding: 12px 14px;
      border-radius: 12px;
      background: #fffdfa;
      border: 1px solid #dbcbb5;
    }
    .account-name {
      font-weight: 700;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      padding: 3px 9px;
      border-radius: 999px;
      background: #eee2d1;
      color: #5a4630;
      font-size: 12px;
      font-weight: 700;
    }
    .badge-current {
      background: #2f2418;
      color: #fff8ed;
    }
    form {
      margin-top: 8px;
    }
    label {
      display: block;
      margin-bottom: 14px;
      font-size: 14px;
      font-weight: 600;
    }
    input {
      width: 100%;
      margin-top: 6px;
      padding: 12px 14px;
      border-radius: 12px;
      border: 1px solid #cdbda6;
      background: #fffdfa;
      box-sizing: border-box;
      font: inherit;
    }
    input:focus {
      outline: 2px solid #b86f37;
      outline-offset: 2px;
    }
    .hint {
      font-size: 13px;
      color: #6b5c49;
      margin-bottom: 18px;
    }
    button {
      width: 100%;
      padding: 12px 16px;
      border: 0;
      border-radius: 999px;
      background: #2f2418;
      color: #fff8ed;
      font: inherit;
      cursor: pointer;
    }
    button:hover {
      background: #433224;
    }
  </style>
</head>
<body>
  <main class="card">
    <h1>{{ .Heading }}</h1>
    <p>{{ .Body }}</p>
    {{ if .Error }}<div class="error">{{ .Error }}</div>{{ end }}
    <h2>Existing accounts</h2>
    <ul class="accounts-list">
      {{ range .Accounts }}
      <li>
        <span class="account-name">{{ .Username }}</span>
        {{ if .IsAdmin }}<span class="badge">Admin</span>{{ end }}
        {{ if .IsCurrent }}<span class="badge badge-current">Current account</span>{{ end }}
      </li>
      {{ end }}
    </ul>
    <h2>Create another local account</h2>
    <form method="post" action="/admin/accounts">
      <label>
        Username
        <input type="text" name="username" autocomplete="username" value="{{ .Username }}" required>
      </label>
      <label>
        Password
        <input type="password" name="password" autocomplete="new-password" required>
      </label>
      <label>
        Confirm password
        <input type="password" name="confirmPassword" autocomplete="new-password" required>
      </label>
      <div class="hint">Use at least {{ .PasswordMinLength }} characters.</div>
      <button type="submit">{{ .SubmitLabel }}</button>
    </form>
  </main>
</body>
</html>
`))
