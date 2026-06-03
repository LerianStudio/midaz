<?xml version="1.0" encoding="UTF-8"?>
<documento codigoDocumento="4111" cnpj="{{ midaz_onboarding.organization.0.legal_document|slice:":8" }}" dataBase="{% date_time "YYYY/MM/dd" %}" tipoRemessa="I">
  <registros>
{% last_item_by_group midaz_transaction.operation group_by "account_id,route" order_by "created_at" if route as listaOperations %}
{%- for oproute in midaz_transaction.operation_route %}
{%- if oproute.code %}
    <registro>
      <conta>{{ oproute.code }}</conta>
      <saldoDia>{% sum_by listaOperations by "available_balance_after" if oproute.id == route %}</saldoDia>
    </registro>
{%- endif %}
{%- endfor %}
  </registros>
</documento>