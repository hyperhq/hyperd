# To run this, make a virtualenv and pip install cryptography.
# If you're using virtualenvwrapper you can just mktmpenv

import datetime
import ipaddress
import uuid

from cryptography import x509
from cryptography.hazmat.backends import default_backend
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import NameOID


one_day = datetime.timedelta(1, 0, 0)
private_key = rsa.generate_private_key(
    public_exponent=65537,
    key_size=2048,
    backend=default_backend()
)
builder = x509.CertificateBuilder()
builder = builder.subject_name(x509.Name([
    x509.NameAttribute(NameOID.COMMON_NAME, u'127.0.0.1'),
]))
builder = builder.issuer_name(x509.Name([
    x509.NameAttribute(NameOID.COMMON_NAME, u'127.0.0.1'),
]))
builder = builder.not_valid_before(datetime.datetime.today() - one_day)
builder = builder.not_valid_after(datetime.datetime(2018, 12, 31))
builder = builder.serial_number(int(uuid.uuid4()))
builder = builder.public_key(private_key.public_key())
builder = builder.add_extension(
    x509.SubjectAlternativeName([
        x509.IPAddress(ipaddress.IPv4Address(u'127.0.0.1'))
    ]),
    critical=False,
)
builder = builder.add_extension(
    x509.KeyUsage(True, False, True, False, True, True, True, False, False),
    critical=False,
)
builder = builder.add_extension(
    x509.BasicConstraints(ca=True, path_length=None),
    critical=False,
)
certificate = builder.sign(
    private_key=private_key, algorithm=hashes.SHA256(),
    backend=default_backend()
)
print(certificate.public_bytes(serialization.Encoding.PEM))
print(private_key.private_bytes(
    serialization.Encoding.PEM,
    serialization.PrivateFormat.PKCS8,
    serialization.NoEncryption()
))
