{% for record in fake_database.nonexistent_table %}
This should fail: {{ record.id }}
{% endfor %}

