{% for account in midaz_onboarding.account %}
Account: {{ account.id }}
Invalid calc: {% calc account.nonexistent_field + 100 %}
Division by zero: {% calc 10 / 0 %}
{% endfor %}

