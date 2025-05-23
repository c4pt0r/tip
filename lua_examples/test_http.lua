local json = require("json")

local event_loop = {
	running = false,
	pending_tasks = 0
}

function event_loop:add_task()
	self.pending_tasks = self.pending_tasks + 1
end

function event_loop:remove_task()
	self.pending_tasks = self.pending_tasks - 1
end

function event_loop:run()
	self.running = true
	while self.running and self.pending_tasks > 0 do
		os.execute("sleep 0.1")
	end
end

local ok, response = http.fetch(
	"GET",
	"https://reqres.in/api/users?page=2",
	{ ["Content-Type"] = "application/json" },
	""
)

if ok then
	print("Sync HTTP GET result:", response.body)
end

event_loop:add_task()
http.fetch(
	"GET",
	"https://reqres.in/api/users?page=2",
	{
		["Content-Type"] = "application/json",
	},
	"",
	function(ok, result)
		if ok then
			print("Async HTTP GET result:", result.body)
		else
			print("Error:", result)
		end
		-- it's safe to call remove_task cuz it's in the main thread
		event_loop:remove_task()
	end
)

print("Starting event loop...")
event_loop:run()
print("Finished!")

-- Synchronous POST example
local post_data = {
	name = "morpheus",
	job = "leader"
}
local post_body = json.encode(post_data)

local ok, response = http.fetch(
	"POST",
	"https://reqres.in/api/users",
	{ ["Content-Type"] = "application/json" },
	post_body
)

if ok then
	print("Sync HTTP POST result:", response.body)
end

-- Asynchronous POST example
event_loop:add_task()
http.fetch(
	"POST",
	"https://reqres.in/api/users",
	{
		["Content-Type"] = "application/json",
	},
	post_body,
	function(ok, result)
		if ok then
			print("Async HTTP POST result:", result.body)
		else
			print("Error:", result)
		end
		event_loop:remove_task()
	end
)

print("Starting event loop for POST requests...")
event_loop:run()
print("Finished POST examples!")
