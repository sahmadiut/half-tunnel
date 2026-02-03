# Security Policy

## Supported Versions

We actively maintain and provide security updates for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue, please report it responsibly.

### How to Report

**Please do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please report security vulnerabilities by emailing the maintainers directly or using GitHub's private vulnerability reporting feature.

1. **GitHub Security Advisory**: Go to the [Security tab](https://github.com/sahmadiut/half-tunnel/security) and click "Report a vulnerability"
2. **Email**: Contact the maintainers directly (see the repository's maintainers)

### What to Include

When reporting a vulnerability, please include:

- Description of the vulnerability
- Steps to reproduce the issue
- Potential impact and severity
- Suggested fix (if any)
- Your contact information for follow-up

### Response Timeline

- **Acknowledgment**: Within 48 hours
- **Initial Assessment**: Within 7 days
- **Resolution Target**: Within 30 days for critical issues

### Disclosure Policy

- We will work with you to understand and resolve the issue
- We ask that you give us reasonable time to address the vulnerability before public disclosure
- We will credit researchers who responsibly disclose vulnerabilities (unless you prefer to remain anonymous)

## Security Best Practices

When deploying Half-Tunnel, follow these security recommendations:

### TLS Configuration

**Always use TLS in production:**

```yaml
server:
  upstream:
    tls:
      enabled: true
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
```

### Authentication

- Enable HMAC authentication for packet integrity
- Use strong, unique session keys
- Consider client certificate authentication for high-security deployments

### Network Security

- Deploy upstream and downstream servers on separate infrastructure
- Use firewalls to restrict access to management ports
- Limit SOCKS5 proxy access to localhost only
- Monitor for unusual traffic patterns

### Configuration Security

- Never commit sensitive configuration to version control
- Use environment variables for secrets
- Restrict file permissions on configuration files
- Regularly rotate encryption keys

### Monitoring

- Enable logging for security events
- Set up alerts for failed authentication attempts
- Monitor session counts for anomalies
- Regularly review access logs

## Known Security Considerations

### Traffic Analysis

Half-Tunnel is designed to obscure traffic analysis by splitting upstream and downstream paths. However:

- Traffic timing correlations may still be possible
- Packet sizes may reveal information about payload types
- Consider additional padding or traffic shaping for high-security use cases

### Session Management

- Session IDs are cryptographically random UUIDs
- Sessions expire after configurable idle timeout
- Reconnection uses the same session ID for continuity

### Encryption

- Default encryption: AES-256-GCM
- Alternative: ChaCha20-Poly1305
- Keys are derived during session handshake

## Security Updates

Security updates are released as:

1. **Patch releases**: For critical vulnerabilities (e.g., v1.0.1)
2. **Security advisories**: Published on GitHub
3. **Announcements**: Posted in release notes

We recommend:

- Subscribing to repository releases
- Enabling Dependabot alerts
- Regularly updating to the latest version

## Audit Status

Half-Tunnel has not undergone a formal third-party security audit. If you're considering using Half-Tunnel in a high-security environment, we recommend:

1. Reviewing the source code
2. Conducting your own security assessment
3. Following the security best practices above

## Contact

For security-related questions that don't involve reporting vulnerabilities, you can open a GitHub discussion.
