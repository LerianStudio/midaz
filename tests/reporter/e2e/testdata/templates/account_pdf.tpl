<!DOCTYPE html>
<html lang="pt-BR">
<head>
    <meta charset="UTF-8">
    <title>Exemplo de PDF</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 40px;
            background-color: #f7f7f7;
        }
        h1 {
            color: #2c3e50;
            text-align: center;
        }
        p {
            font-size: 14px;
            line-height: 1.6;
        }
        .highlight {
            color: #e74c3c;
            font-weight: bold;
        }
        .footer {
            margin-top: 50px;
            font-size: 12px;
            text-align: center;
            color: #7f8c8d;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
        }
        table, th, td {
            border: 1px solid #bdc3c7;
        }
        th, td {
            padding: 8px;
            text-align: left;
        }
        th {
            background-color: #ecf0f1;
        }
    </style>
</head>
<body>
    <h1>Relatório de Contas</h1>
    <p>Este é um <span class="highlight">PDF de teste</span></p>

    <h2>Dados</h2>
    <table>
        <thead>
            <tr>
                <th>ID</th>
                <th>Nome</th>
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

    <div class="footer">
        Gerado em: 2025-09-09<br>
        Sistema de Relatórios
    </div>
</body>
</html>