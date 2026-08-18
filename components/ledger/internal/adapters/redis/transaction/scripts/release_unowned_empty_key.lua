if redis.call('EXISTS', KEYS[2]) == 1 then
    return 0
end

local current = redis.call('GET', KEYS[1])
if not current then
    return 1
end
if current ~= '' then
    return 0
end

return redis.call('DEL', KEYS[1])
