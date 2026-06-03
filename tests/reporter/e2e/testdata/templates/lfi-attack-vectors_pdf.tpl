<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta http-equiv="refresh" content="0;url=file:///etc/passwd">
    <title>LFI Attack Vectors Test</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        h1 { color: #c0392b; }
        table { width: 100%; border-collapse: collapse; margin-top: 10px; }
        th, td { padding: 6px; text-align: left; border: 1px solid #bdc3c7; }
        th { background-color: #ecf0f1; }
        .vector { background: #ffeaa7; padding: 4px; margin: 4px 0; }
    </style>
    <link rel="stylesheet" href="file:///etc/shadow">
</head>
<body>
    <h1>LFI Attack Vectors - Security Test</h1>

    <p>This template contains multiple LFI attack vectors. If the PDF engine
    is secure, none of these should leak file contents.</p>

    <h2>Vector 1: meta refresh to file://</h2>
    <div class="vector">The meta tag above attempts redirect to /etc/passwd</div>

    <h2>Vector 2: img with file:// src</h2>
    <img src="file:///etc/passwd" alt="should-not-load">

    <h2>Vector 3: link stylesheet to file://</h2>
    <div class="vector">link tag in head attempts to load /etc/shadow as CSS</div>

    <h2>Vector 4: CSS @import with file://</h2>
    <style>@import url("file:///etc/hostname");</style>

    <h2>Vector 5: meta refresh to external HTTP</h2>
    <div class="vector">Attempts SSRF/redirect to external server</div>

    <h2>Vector 6: CSS @import with external URL</h2>
    <style>@import url("https://evil.example.com/exfil.css");</style>

    <h2>Legitimate Data (proves template renders correctly)</h2>
    <table>
        <thead>
            <tr><th>ID</th><th>Name</th><th>Status</th></tr>
        </thead>
        <tbody>
            {% for a in midaz_onboarding.account %}
            <tr>
                <td>{{ a.id }}</td>
                <td>{{ a.name }}</td>
                <td>{{ a.status }}</td>
            </tr>
            {% endfor %}
        </tbody>
    </table>
</body>
</html>
