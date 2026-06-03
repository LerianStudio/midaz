<!DOCTYPE html>
<html>
<head><title>Event Handler XSS Test</title></head>
<body>
    <h1>Report</h1>
    <img src="x" onerror="alert(1)">
    {% for org in midaz_onboarding.organization %}
    <p>{{ org.name }}</p>
    {% endfor %}
</body>
</html>