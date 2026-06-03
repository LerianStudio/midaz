{%- for account in midaz_onboarding.account -%}
{%- with balance = filter(midaz_transaction.balance, "account_id", account.id)[0] %}
{%- if balance.available > 0 %}
{{ account.id }}, {{ account.alias }} - Conta Ativa
{%- else %}
{{ account.id }}, {{ account.alias }} - Conta Zerada
{%- endif %}
{%- endwith %}
{%- endfor %}