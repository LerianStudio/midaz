<!DOCTYPE html>
<html>
<head>
    <title>Account Listing</title>
    <style>
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
        }
        th, td {
            padding: 8px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background-color: #f2f2f2;
            font-weight: bold;
        }
        tr:hover {
            background-color: #f5f5f5;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        h1 {
            color: #333;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Account Listing</h1>

        {% if midaz_onboarding.account %}
            <table>
                <thead>
                    <tr>
                        <th>ID</th>
                        <th>Alias</th>
                        <th>Description</th>
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
        {% else %}
            <p>No accounts found.</p>
        {% endif %}
    </div>
</body>
</html>