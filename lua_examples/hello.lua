local ret = sql.query('select "hello world from lua"')

if ret.ok then
	print(ret.data[1][1])
end
