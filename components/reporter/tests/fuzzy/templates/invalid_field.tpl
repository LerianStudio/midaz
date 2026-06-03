{% for account in midaz_onboarding.account %}
Account: {{ account.id }}
Non-existent field: {{ account.this_field_does_not_exist }}
Another invalid: {{ account.missing_column }}
{% endfor %}

