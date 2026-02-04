## Logs to DB 
This module monitors log files and load its contents into database.

### Functional Requirements
 - It reads log entries from the monitored log files and loads them into the specified database table.
 - There can be multiple log files. All log files are in the same directory. The log file directory is specified in the config file. 
 - The location of the config file is specified by the env variable LOG2DB_CONFIG
 - This module is a service.
 - The service needs to keep track of the log entries it has already loaded
 - It loads log entries at the specified frequency (in seconds)

### Environment Variables
|Name | Explanation |
|:----|:------------|
|LOG2DB_CONFIG | The config file name (absolute path) or relative to $HOME

### Configuration
It is a .toml file:
```text
log_file_dir = "/path/to/your/logfiledir"
db_table_name = "tablename"
log_entry_format = "json"
sync_freq_in_secon = <integer>, default to 10 seconds

[json-mapping]
entry_type = "_meta.logLevelName"
message = "2"
sys_prompt = "1.lines"
sys_prompt_nlines = "1.lineCount"
caller_filename = "1.callerFile"
caller_line = "1.callerLine"
created_at = "time"
```

### Programming Language
 - Backend use Go 
 - Frontend use TypeScript and Svelte

### Log File Format 
Log entries are JSON objects. Each line defines one JSON object. Below is an example of the JSON object:
```json
{
	"0":"{\"subsystem\":\"agent-system-prompt\"}",
	"1":{
		"lineCount":133,
		"lines":["You are a personal assistant running inside OpenClaw.",
		"",
		"## Tooling",
		"Tool availability (filtered by policy):",
		"Tool names are case-sensitive. Call tools exactly as listed.",
		],
		"callerFile":"file:///Users/cding/Workspace/openclaw/openclaw/dist/agents/system-prompt.js",
		"callerLine":446
	},
	"2":"Built system prompt lines (CLW_02021406)",
	"_meta":{
		"runtime":"node",
		"runtimeVersion":"24.13.0",
		"hostname":"unknown",
		"name":"{\"subsystem\":\"agent-system-prompt\"}",
		"parentNames":["openclaw"],
		"date":"2026-02-03T12:07:12.525Z",
		"logLevelId":2,
		"logLevelName":"DEBUG",
		"path":{
			"fullFilePath":"file:///Users/cding/Workspace/openclaw/openclaw/dist/logging/subsystem.js:229:16",
			"fileName":"subsystem.js",
			"fileNameWithLine":"subsystem.js:229",
			"fileColumn":"16",
			"fileLine":"229",
			"filePath":"dist/logging/subsystem.js",
			"filePathWithLine":"dist/logging/subsystem.js:229",
			"method":"logToFile"
		}
	},
	"time":"2026-02-03T12:07:12.525Z"
}
```

### Table Schema
|Field name | Data Type | Default | Remarks |
|:----------|:----------|:--------|:--------|
| id | varchar(40) | uuid7 | DB automatically generated ID that uniquely identifies records|
| entry_type| varchar(20) | Not Null | enum: "DEBUG", "INFO", "WARN", "ERROR", "FATAL". It is extracted from "_meta.logLevelName" in the JSON object. |
| message | text | Not Null | The message, extracted from the attribute "2" |
| sys_prompt | text | Null | The prompt, extracted from 1.lines |
| sys_prompt_nlines | int | Null | The number of lines in sys_prompt, extracted from 1.lineCount |
| caller_filename | varchar(120) | Null | The name of the file where the log entry is created, extracted from 1.callerFile |
| caller_line | int | Null | The line number at which the log entry is created, extracted from 1.callerLine |
| json_obj | JSONB | Not Null | The JSON object for the entry|
| log_filename | text | Not Null | The name of the log file |
| log_line_num | int | Not Null | The line number of the log entry in the log file |
| error_msg | text | Null | The errors during parsing and constructing this record, if any|
| remarks | text | Null | reserved for human comments |
| created_at | timestamp | Not Null | The creation time of the record, extracted from "time" attribute |

### Source Code Files  

### Log Loader as Service
It should be implemented as a service. It supports the following commands:
| Command | Explanation |
|:--------|:--------|
| log2db start | It starts the service |
| log2db stop | It stops the service |
| log2db status | It returns the service status. Refer below for the status output format |
| log2db reload | It first prompts the usr to clear the table, and then reload all the log files |
| log2db purge -maxfiles [integer] | It keeps up to [integer] most recent log files and delete all other log files, provided they have been loaded into the database.|

### Status Format 
Service Status: (active, not started)
Start Time: the time when the service was started
Total Log Entries: the total number of log entries loaded into the table 
Entries Since Start: the total number of log entries since the the start of the current invovation
Total Errors: number of errors occurred during this invocation

