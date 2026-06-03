<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Account Report - PDF</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        h1 { color: #2c3e50; text-align: center; }
        table { width: 100%; border-collapse: collapse; margin-top: 20px; }
        th, td { padding: 8px; text-align: left; border: 1px solid #bdc3c7; }
        th { background-color: #ecf0f1; }
    </style>
</head>
<body>
    <h1>Account Report</h1>
    <table>
        <thead>
            <tr>
                <th>ID</th>
                <th>Name</th>
                <th>Alias</th>
                <th>Created At</th>
            </tr>
        </thead>
        <tbody>
            {% for a in midaz_onboarding.account %}
            <tr>
                <td>{{ a.id }}</td>
                <td>{{ a.name }}</td>
                <td>{{ a.alias }}</td>
                <td>{{ a.created_at|date:"2006-01-02 15:04:05" }}</td>
            </tr>
            {% endfor %}
        </tbody>
    </table>
</body>
</html>