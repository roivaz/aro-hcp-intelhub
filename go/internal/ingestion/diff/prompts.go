package diff

const mapPromptTemplate = `You are a code analysis tool. Analyze the diff chunk below and report concrete, observable code changes.

Context:
- Pull request title: {{.PRTitle}}
- File path: {{.FilePath}}

Rules:
- Only report facts directly visible in the diff (lines starting with '+' or '-').
- Never speculate or use words like "likely", "suggests", "appears", or "possibly".
- Each bullet must include a quoted snippet from the diff showing the change.
- Output exactly one bullet per distinct change, using the format:
  - [FILE: {{.FilePath}}] <concise description> — "<diff snippet>"
- Maximum 4 bullets; each under 20 words.

<diff>
{{.Text}}
</diff>

**Observed Changes:**
- [FILE: {{.FilePath}}] ...
- [FILE: {{.FilePath}}] ...
- [FILE: {{.FilePath}}] ...
- [FILE: {{.FilePath}}] ...`

const reducePromptTemplate = `You are a technical summarizer. Your task is to analyze the provided Pull Request context and create a factual, concise, and structured summary of the changes.

## Rules:
1.  **Extract, Don't Infer:** Only report on changes explicitly mentioned in the context. Do not invent goals or risks.
2.  **Be Direct and Factual:** Use clear, technical language. Avoid buzzwords.
3.  **Use the Provided Structure:** Fill in the sections below.

**CONTEXT:**

**PR Title:**
{{.PRTitle}}

**PR Description:**
{{.PRDescription}}

**Summaries of Code Changes:**
{{.Text}}

---
**FACTUAL CHANGE SUMMARY:**

### 1. Stated Purpose
(Summarize the goal from the PR Title and Description in 1-2 sentences.)

### 2. Observed Code Changes
(Create a bulleted list of the most significant technical modifications based *only* on the provided code change summaries.)
- 
- 
- `
