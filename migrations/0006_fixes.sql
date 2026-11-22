-- Forwarders and the default address were controlled by the "email" feature
-- until they got switches of their own; keep them off where email was off.
UPDATE feature_lists SET disabled = disabled || ' forwarders'
    WHERE (' ' || disabled || ' ') LIKE '% email %' AND (' ' || disabled || ' ') NOT LIKE '% forwarders %';
UPDATE feature_lists SET disabled = disabled || ' default_address'
    WHERE (' ' || disabled || ' ') LIKE '% email %' AND (' ' || disabled || ' ') NOT LIKE '% default_address %';
