{% for org in midaz_onboarding.organization %}
Organization: {{ org.id }}
{% for account in org.accounts_that_dont_exist %}
  Account: {{ account.invalid_field }}
{% endfor %}
{% endfor %}

