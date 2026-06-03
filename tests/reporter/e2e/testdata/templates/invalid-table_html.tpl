<!DOCTYPE html>
<html>
<head><title>Invalid Table Test</title></head>
<body>
    {% for item in midaz_onboarding.nonexistent_table %}
    <p>{{ item.name }}</p>
    {% endfor %}
</body>
</html>