CREATE TABLE IF NOT EXISTS processing_state (
  id INT PRIMARY KEY DEFAULT 1,
  last_pr_number INT,
  last_pr_timestamp TIMESTAMPTZ
);

