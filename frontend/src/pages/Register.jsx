import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { authAPI } from '../services/api';

const Register = () => {
  const [formData, setFormData] = useState({
    username: '',
    password: '',
    email: '',
    role: 'customer',
  });
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const handleChange = (event) => {
    const { name, value } = event.target;
    setFormData((prev) => ({
      ...prev,
      [name]: value,
    }));
  };

  const handleSubmit = async (event) => {
    event.preventDefault();
    setError('');
    setLoading(true);

    try {
      const data = await authAPI.register(formData);
      localStorage.setItem('token', data.token);
      localStorage.setItem('user_id', data.user_id);
      localStorage.setItem('role', data.role);
      navigate('/');
    } catch (submitError) {
      setError(submitError.response?.data?.error || '注册失败，请稍后重试');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="auth-container">
      <section className="auth-intro">
        <p className="eyebrow">Create Account</p>
        <h1>从浏览商品，到真正走完一次交易。</h1>
        <p>普通用户可下单体验，商家用户可进入控制台维护自己的店铺与商品。</p>
      </section>

      <section className="auth-card">
        <div className="auth-card-header">
          <h2>注册账户</h2>
          <p>选择适合的角色，开始使用 Go Commerce。</p>
        </div>
        {error && <div className="error-message">{error}</div>}
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor="username">用户名</label>
            <input id="username" name="username" value={formData.username} onChange={handleChange} required />
          </div>
          <div className="form-group">
            <label htmlFor="email">邮箱</label>
            <input id="email" name="email" type="email" value={formData.email} onChange={handleChange} required />
          </div>
          <div className="form-group">
            <label htmlFor="password">密码</label>
            <input id="password" name="password" type="password" value={formData.password} onChange={handleChange} required />
          </div>
          <div className="form-group">
            <label htmlFor="role">账户类型</label>
            <select id="role" name="role" value={formData.role} onChange={handleChange}>
              <option value="customer">普通用户</option>
              <option value="merchant">商家用户</option>
            </select>
          </div>
          <button type="submit" className="btn btn-primary" disabled={loading}>
            {loading ? '注册中...' : '注册'}
          </button>
        </form>
        <div className="auth-footer">
          已有账户？<Link to="/login">登录</Link>
        </div>
      </section>
    </div>
  );
};

export default Register;
