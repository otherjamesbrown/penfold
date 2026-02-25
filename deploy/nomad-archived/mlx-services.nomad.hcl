# RETIRED: Replaced by Ollama on dev01:11434 (launchd service).
# See pf-949cf7 for migration details.
# This file is kept for reference only — the Nomad job has been purged.
#
# To re-deploy embeddings, Ollama is managed via launchd:
#   launchctl kickstart user/501/com.ollama.serve
#   curl http://dev01.brown.chat:11434/v1/embeddings -d '{"model":"mxbai-embed-large","input":"test"}'
