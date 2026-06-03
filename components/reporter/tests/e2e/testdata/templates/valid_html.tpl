<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Organization Report</title>
    <style>
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 8px; text-align: left; border: 1px solid #ddd; }
        th { background-color: #f2f2f2; }
    </style>
</head>
<body>
    <h1>Organization Report</h1>
    <table>
        <thead>
            <tr>
                <th>ID</th>
                <th>Name</th>
                <th>Status</th>
                <th>Created At</th>
            </tr>
        </thead>
        <tbody>
            {% for org in midaz_onboarding.organization %}
            <tr>
                <td>{{ org.id }}</td>
                <td>{{ org.name }}</td>
                <td>{{ org.status }}</td>
                <td>{{ org.created_at|date:"2006-01-02" }}</td>
            </tr>
            {% endfor %}
        </tbody>
    </table>
</body>
</html>