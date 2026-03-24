Use superpowers to create a PDF parser and save it in shared/go/api/parsers/pdf-parser:

- PDF Parser is a service, written in Go.
- The service uses PaddleORC (https://github.com/PaddlePaddle/PaddleOCR.git) to parse PDF docs.  Note that Paddle OCR is alredy installed in ~/Workspace/ThirdParty/paddleocr (in the subdirectory PaddleOCR)
- There is a database table: 'kb.inputs' that manages all the inputs. Each record in the table is an input, which can be a document (such as PDF, Word, Excel, PPT, text, markdown, etc.). The table has the following fields:
| Field Name | Required | Explanation |
|:-----------|:---------|:------------|
| id | mandatory | Auto-incremented ID (integer) that identifies the record |
| name | optional | The name of the inputs |
| type | mandatory | The input type, such as 'word', 'pdf', ... |
| title | optional | The title if the input is a document |
| doc_no | optional | The document number, which is normally a string, if any |
| source | optional | Where the input came from |
| file_name | optional | The file name (url) at which the file is stored, applicable to files only |
| backup_filename | optional | The backup file name |
| publish_date | optional | The doc's publish date |
| authors | optional | The doc's authors |
| owner | optional | The ID of the user Who owns the input |
| status | mandatory | A JSON doc that keeps track of the operations on the input (refer below to its definition) |
| create_time | mandatory | The creation time (read-only) |
| modify_time | mandatory | The last modification time |
| public_info | optional | A JSON document that stores additional public info |
| private_info | optional | A JSON document that stores additional private info |
| notes | optional | Stores notes |
| error_msg | optional | Stores error messages, such as processing error messages |
- Status: various processing may be done on an input, such as parsing (for PDF, Word, etc.), analyzing, adding to knowledge, etc. Each operation is recorded as "operation": the type of the operation, "time": the time when the operation was performed, "status": success or failed, and "error": the error message. This field is a JSON of the following format:
[
    {"operation":"the-opr", "time":"timestamp-in-yyyymmdd hh:mm:ss", "status":"success or fail", "error":"error-msg"},
    {"operation":"the-opr", "time":"timestamp-in-yyyymmdd hh:mm:ss", "status":"success or fail", "error":"error-msg"},
    ...
]
- The service monitors the database table 'kb.inputs'. For 'type' = 'pdf' input, if its status does not have an entry whose "operation" is "parse", it will pick up the record and start parsing the doc.
- There are three directories: staging directory, repo directory, and backup directory. Input files are originally saved in the staging directory. After being processed, it is moved to the repo directory and the backup directory. The backup directory can be on a remote machine.
- There can be multiple repo directories. When copying a file to the the file repo, the service should pick the least used repo directory, if multiple repo directories are configured.
- After parsing a PDF file, this service will update the field 'status', copy the file to a repo directory, and back up the file to the backup directory
- There is an example file: /Users/cding/Workspace/ThirdParty/paddleocr/parse_pdf.py that shows how to use PaddleOCR to parse a PDF file.