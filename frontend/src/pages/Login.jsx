import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { authAPI } from '../services/api';

const Login = () => {
  const [formData, setFormData] = useState({ username: '', password: '' });
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const handleChange = (event) => {
    const { name, value } = event.target;
    setFormData((prev) => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (event) => {
    event.preventDefault();
    setError('');
    setLoading(true);

    try {
      const data = await authAPI.login(formData);
      localStorage.setItem('token', data.token);
      localStorage.setItem('user_id', data.user_id);
      localStorage.setItem('role', data.role);
      if (data.role === 'merchant') {
        navigate('/merchant');
      } else if (data.role === 'admin') {
        navigate('/merchants');
      } else {
        navigate('/');
      }
    } catch (submitError) {
      setError(submitError.response?.data?.error || '登录失败，请检查用户名和密码');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="auth-container">
      <section className="auth-intro">
        <p className="eyebrow">Welcome Back</p>
        <h1>继续浏览、下单和管理你的店铺。</h1>
        <p>登录后可查看购物车、订单，并根据角色进入商家后台或商户管理。</p>
      </section>

      <section className="auth-card">
        <div className="auth-card-header">
          <h2>用户登录</h2>
          <p>使用你的账户继续体验完整交易流程。</p>
        </div>
        {error && <div className="error-message">{error}</div>}
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor="username">用户名</label>
            <input id="username" name="username" value={formData.username} onChange={handleChange} required />
          </div>
          <div className="form-group">
            <label htmlFor="password">密码</label>
            <input id="password" name="password" type="password" value={formData.password} onChange={handleChange} required />
          </div>
          <button type="submit" className="btn btn-primary" disabled={loading}>
            {loading ? '登录中...' : '登录'}
          </button>
        </form>
        <div className="auth-footer">
          没有账户？<Link to="/register">注册</Link>
        </div>
      </section>
    </div>
  );
};

export default Login;
