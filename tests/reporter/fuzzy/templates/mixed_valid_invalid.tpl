{% for org in midaz_onboarding.organization %}
Valid: {{ org.id }}
Valid: {{ org.name }}
Invalid: {{ org.this_does_not_exist }}
Valid: {{ org.created_at }}
Invalid: {{ org.fake_field }}
{% endfor %}

