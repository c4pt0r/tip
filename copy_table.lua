local original_table = args[1]
local new_table = args[2]

if not original_table then
	print("source table name is missing")
	return -1
end

if not new_table then
	print("target table name is missing")
	return -1
end

local ret = sql.query("SHOW CREATE TABLE " .. original_table)
if ret.ok then
	local create_stmt = ret.data[1][2]
	local new_create = create_stmt:gsub(original_table, new_table)
	local create_ret = sql.execute(new_create)
	if not create_ret.ok then
		print("create new table failed: " .. create_ret.error)
		return
	end
else
	print("get original table failed: " .. ret.error)
	return
end

local insert_ret = sql.execute("INSERT INTO " .. new_table .. " SELECT * FROM " .. original_table)
if not insert_ret.ok then
	print("failed: " .. insert_ret.error)
else
	print("success: " .. new_table)
end

return 0
