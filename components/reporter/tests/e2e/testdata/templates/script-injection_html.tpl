<!DOCTYPE html>
<html>
<head><title>XSS Test</title></head>
<body>
    <h1>Report</h1>
    <script>alert('xss')</script>
    {% for org in midaz_onboarding.organization %}
    <p>{{ org.name }}</p>
    {% endfor %}
</body>
</html>