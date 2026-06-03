{% for a in data.table1 %}
  {% for b in data.table2 %}
    {% for c in data.table3 %}
      {% for d in data.table4 %}
        {% for e in data.table5 %}
          {{ e.field }}
        {% endfor %}
      {% endfor %}
    {% endfor %}
  {% endfor %}
{% endfor %}

