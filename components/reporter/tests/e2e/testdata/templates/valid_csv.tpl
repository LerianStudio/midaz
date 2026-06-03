id,name,status,created_at
{% for org in midaz_onboarding.organization %}{{ org.id }},{{ org.name }},{{ org.status }},{{ org.created_at|date:"2006-01-02" }}
{% endfor %}