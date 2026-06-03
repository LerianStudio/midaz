<script>alert('XSS attempt')</script>
{% for account in midaz_onboarding.account %}
{{ account.id }}
<script>console.log('{{ account.id }}')</script>
{% endfor %}

