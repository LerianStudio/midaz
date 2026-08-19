if redis.call("ZCARD", KEYS[1]) > 0 then
    return 1
end
if redis.call("SCARD", KEYS[2]) > 0 then
    return 1
end
return 0
