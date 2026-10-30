"""Downloads one Hugging Face model into HF_HOME for TARS (internal/localembed)."""

import sys

from huggingface_hub import snapshot_download

print(snapshot_download(sys.argv[1]), flush=True)
