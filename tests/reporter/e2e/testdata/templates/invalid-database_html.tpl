<!DOCTYPE html>
<html>
<head><title>Invalid Database Test</title></head>
<body>
    {% for item in nonexistent_db.some_table %}
    <p>{{ item.name }}</p>
    {% endfor %}
</body>
</html>