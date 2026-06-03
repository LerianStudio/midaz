<?xml version="1.0" encoding="UTF-8"?>
<CCSDOC xmlns="http://www.bcb.gov.br/ccs/ACCS005.xsd">
    <BCARQ>
        <IdentdEmissor>12345678</IdentdEmissor>
        <IdentdDestinatario>00000001</IdentdDestinatario>
        <NomArq>ACCS005</NomArq>
        <NumRemessaArq>{% date_time "YYYYMMddHHmm" %}</NumRemessaArq>
    </BCARQ>
    <SISARQ>
        <CCSArqInfDettRelctPessoa>
            <CNPJBaseEntRespons>{{plugin_crm.holders.0.document|slice:':8'}}</CNPJBaseEntRespons>
            <NumCtrlCCSOr>000000</NumCtrlCCSOr>
            <Repet_CCS0005_BDV>
                {%- for account in midaz_onboarding.account %}
                {%- for alias in plugin_crm.aliases %}
                {%- if alias.account_id == account.id %}
                {%- for holder in plugin_crm.holders %}
                {%- if holder.document == alias.document %}
                <Grupo_CCS0005_BDV>
                    <SitInfDettPessoa></SitInfDettPessoa>
                    <Grupo_CCS0005_Part>
                        <CNPJBasePart>{{holder.document|slice:"0:8"}}</CNPJBasePart>
                        <TpBDV>{% if account.type == "deposit" %}1{% elif account.type == "savings" %}2{% elif account.type == "payment" %}3{% else %}6{% endif %}</TpBDV>
                        {%- if alias.banking_details.branch %}
                        <AgIF>{{alias.banking_details.branch}}</AgIF>
                        {%- endif %}
                        {%- if alias.banking_details.type == "CACC" %}
                        <CtCli>{{alias.banking_details.account}}</CtCli>
                        {%- elif alias.banking_details.type == "payment" %}
                        <CtPgto>{{alias.banking_details.account}}</CtPgto>
                        {%- endif %}
                        <TpVincBDV>1</TpVincBDV>
                        <NomPessoa>{{holder.name}}</NomPessoa>
                        <DtIni>{{alias.banking_details.opening_date}}</DtIni>
                        <DtFim></DtFim>
                        {%- if holder.type == "NATURAL_PERSON" and holder.natural_person %}
                        <Repet_CCS0005_Vincd>
                            <Grupo_CCS0005_Vincd>
                                <Grupo_CCS0005_PessoaVincd>
                                    <TpVinc>3</TpVinc>
                                    <CNPJ_CPFPessoaVincd>{{holder.document}}</CNPJ_CPFPessoaVincd>
                                    <NomPessoaVincd>{{holder.natural_person.mother_name}}</NomPessoaVincd>
                                    <DtIniVinc>{{alias.banking_details.opening_date}}</DtIniVinc>
                                    <DtFimVinc></DtFimVinc>
                                </Grupo_CCS0005_PessoaVincd>
                            </Grupo_CCS0005_Vincd>
                        </Repet_CCS0005_Vincd>
                        {%- endif %}
                    </Grupo_CCS0005_Part>
                </Grupo_CCS0005_BDV>
                {%- endif %}
                {%- endfor %}
                {%- endif %}
                {%- endfor %}
                {%- endfor %}
            </Repet_CCS0005_BDV>
            <DtMovto>{% date_time "YYYY-MM-DD" %}</DtMovto>
        </CCSArqInfDettRelctPessoa>
    </SISARQ>
</CCSDOC>
