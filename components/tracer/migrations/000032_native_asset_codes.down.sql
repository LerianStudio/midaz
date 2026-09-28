-- Refuse rollback while any stored limit code does not fit the previous
-- three-character column, or is a three-character code the previous binary
-- rejects: that binary accepted only ISO 4217 codes. The frozen list matches the
-- ISO 4217 set that binary validated against, including historical codes, not
-- only currently circulating currencies. Never truncate: a shortened code would
-- silently change which asset a limit refers to.
SET LOCAL lock_timeout = '5s';

DO $migration$
BEGIN
    LOCK TABLE limits IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT 1 FROM limits WHERE length(asset) <> 3) THEN
        RAISE EXCEPTION 'cannot restore the three-character limits.asset column while longer or shorter asset codes are stored'
            USING ERRCODE = '23514';
    END IF;
    IF EXISTS (SELECT 1 FROM limits WHERE NOT (asset = ANY (ARRAY[
        'ADP','AED','AFA','AFN','ALK','ALL','AMD','ANG','AOA','AOK','AON','AOR','ARA','ARL','ARM','ARP','ARS','ATS','AUD','AWG','AZM','AZN','BAD','BAM','BAN','BBD','BDT','BEC','BEF','BEL','BGL','BGM','BGN','BGO','BHD','BIF','BMD','BND','BOB','BOL','BOP','BOV','BRB','BRC','BRE','BRL','BRN','BRR','BRZ','BSD','BTN','BUK','BWP','BYB','BYN','BYR','BZD','CAD','CDF','CHE','CHF','CHW','CLE','CLF','CLP','CNH','CNX','CNY','COP','COU','CRC','CSD','CSK','CUC','CUP','CVE','CYP','CZK','DDM','DEM','DJF','DKK','DOP','DZD','ECS','ECV','EEK','EGP','ERN','ESA','ESB','ESP','ETB','EUR','FIM','FJD','FKP','FRF','GBP','GEK','GEL','GHC','GHS','GIP','GMD','GNF','GNS','GQE','GRD','GTQ','GWE','GWP','GYD','HKD','HNL','HRD','HRK','HTG','HUF','IDR','IEP','ILP','ILR','ILS','INR','IQD','IRR','ISJ','ISK','ITL','JMD','JOD','JPY','KES','KGS','KHR','KMF','KPW','KRH','KRO','KRW','KWD','KYD','KZT','LAK','LBP','LKR','LRD','LSL','LTL','LTT','LUC','LUF','LUL','LVL','LVR','LYD','MAD','MAF','MCF','MDC','MDL','MGA','MGF','MKD','MKN','MLF','MMK','MNT','MOP','MRO','MTL','MTP','MUR','MVP','MVR','MWK','MXN','MXP','MXV','MYR','MZE','MZM','MZN','NAD','NGN','NIC','NIO','NLG','NOK','NPR','NZD','OMR','PAB','PEI','PEN','PES','PGK','PHP','PKR','PLN','PLZ','PTE','PYG','QAR','RHD','ROL','RON','RSD','RUB','RUR','RWF','SAR','SBD','SCR','SDD','SDG','SDP','SEK','SGD','SHP','SIT','SKK','SLL','SOS','SRD','SRG','SSP','STD','STN','SUR','SVC','SYP','SZL','THB','TJR','TJS','TMM','TMT','TND','TOP','TPE','TRL','TRY','TTD','TWD','TZS','UAH','UAK','UGS','UGX','USD','USN','USS','UYI','UYP','UYU','UZS','VEB','VEF','VND','VNN','VUV','WST','XAF','XAG','XAU','XBA','XBB','XBC','XBD','XCD','XDR','XEU','XFO','XFU','XOF','XPD','XPF','XPT','XRE','XSU','XTS','XUA','XXX','YDD','YER','YUD','YUM','YUN','YUR','ZAL','ZAR','ZMK','ZMW','ZRN','ZRZ','ZWD','ZWL','ZWR'
    ]::text[]))) THEN
        RAISE EXCEPTION 'cannot restore the ISO 4217-only limits.asset column while non-ISO asset codes are stored'
            USING ERRCODE = '23514';
    END IF;

    ALTER TABLE limits DROP CONSTRAINT limits_asset_code_format;
    ALTER TABLE limits ALTER COLUMN asset TYPE VARCHAR(3);
END;
$migration$;
