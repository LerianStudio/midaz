<?xml version="1.0" encoding="UTF-8"?>
<organizations>
    {% for org in midaz_onboarding.organization %}
    <organization>
        <id>{{ org.id }}</id>
        <name>{{ org.name }}</name>
        <status>{{ org.status }}</status>
        <createdAt>{{ org.created_at|date:"2006-01-02" }}</createdAt>
    </organization>
    {% endfor %}
</organizations>