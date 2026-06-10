<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Multi-Source Report</title>
    <style>
        table { width: 100%; border-collapse: collapse; margin-top: 10px; }
        th, td { padding: 6px; text-align: left; border: 1px solid #ddd; }
        th { background-color: #f2f2f2; }
    </style>
</head>
<body>
    <h1>Multi-Source Report</h1>

    <h2>Organizations (PostgreSQL)</h2>
    <table>
        <thead>
            <tr>
                <th>ID</th>
                <th>Name</th>
                <th>Status</th>
            </tr>
        </thead>
        <tbody>
            {% for org in midaz_onboarding.organization %}
            <tr>
                <td>{{ org.id }}</td>
                <td>{{ org.name }}</td>
                <td>{{ org.status }}</td>
            </tr>
            {% endfor %}
        </tbody>
    </table>

    <h2>Holders (MongoDB)</h2>
    <table>
        <thead>
            <tr>
                <th>ID</th>
                <th>Email</th>
                <th>Status</th>
                <th>Type</th>
            </tr>
        </thead>
        <tbody>
            {% for h in crm.holders %}
            <tr>
                <td>{{ h._id }}</td>
                <td>{{ h.email }}</td>
                <td>{{ h.status }}</td>
                <td>{{ h.type }}</td>
            </tr>
            {% endfor %}
        </tbody>
    </table>
</body>
</html>