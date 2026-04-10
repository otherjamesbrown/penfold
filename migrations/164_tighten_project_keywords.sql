-- +goose Up

-- Healthcare: remove bare "appointment" (matches webuyanycar) and "surgery" (ambiguous)
UPDATE projects SET keywords = ARRAY[
    'doctor', 'dentist', 'dental', 'GP surgery', 'NHS',
    'prescription', 'pharmacy', 'clinic', 'hospital',
    'appointment reminder', 'medical', 'consultation',
    'orthotic', 'physio', 'physiotherapy', 'x-ray',
    'referral', 'patient'
] WHERE name = 'Healthcare';

-- Cars: remove bare "insurance" (overlaps Finance)
UPDATE projects SET keywords = ARRAY[
    'car', 'vehicle', 'motor', 'MOT', 'autotrader', 'cazoo',
    'cinch', 'DVLA', 'V5', 'logbook', 'Audi', 'Porsche',
    'webuyanycar', 'car insurance', 'breakdown cover'
] WHERE name = 'Cars';

-- Finance: remove bare "insurance"
UPDATE projects SET keywords = ARRAY[
    'bank statement', 'transaction', 'investment', 'pension',
    'mortgage', 'tax', 'HMRC', 'salary', 'payslip',
    'direct debit', 'standing order', 'ISA', 'savings account'
] WHERE name = 'Finance';

-- Home: remove generic "order" (matches every e-commerce email)
UPDATE projects SET keywords = ARRAY[
    'delivery notification', 'tracking number', 'octopus energy',
    'amazon order', 'evri', 'royal mail', 'parcel tracking',
    'energy bill', 'council tax', 'broadband', 'utility'
] WHERE name = 'Home';

-- Family
UPDATE projects SET keywords = ARRAY[
    'Ayla', 'mum', 'dad', 'brother', 'sister',
    'nephew', 'niece', 'school', 'nursery', 'parents evening'
] WHERE name = 'Family';

-- AI: remove bare "AI" (false positives)
UPDATE projects SET keywords = ARRAY[
    'artificial intelligence', 'LLM', 'GPT-4', 'GPT-5',
    'ChatGPT', 'Claude', 'OpenAI', 'Anthropic', 'Gemini',
    'machine learning', 'deep learning', 'transformer model',
    'prompt engineering', 'RAG', 'vector database'
] WHERE name = 'AI';

-- Job Search: remove generic "role"
UPDATE projects SET keywords = ARRAY[
    'LinkedIn', 'hiring manager', 'recruiter', 'job vacancy',
    'interview', 'CV review', 'job application', 'job alert',
    'job offer'
] WHERE name = 'Job Search';

-- +goose Down
-- No down migration — original keywords were the bug we're fixing
SELECT 1;
