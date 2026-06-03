{% for account in midaz_onboarding.account %}
{%- with balance = filter(midaz_transaction.balance, "account_id_wrong", account.id)[0] %}
Balance: {{ balance.available }}
{% endwith %}
{% endfor %}

