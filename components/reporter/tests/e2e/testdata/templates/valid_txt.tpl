Organization Report
====================
{% for org in midaz_onboarding.organization %}
ID: {{ org.id }}
Name: {{ org.name }}
Status: {{ org.status }}
Created: {{ org.created_at|date:"2006-01-02" }}
---
{% endfor %}