import { Link, useNavigate } from 'react-router-dom';

const Navbar = () => {
  const navigate = useNavigate();
  const token = localStorage.getItem('token');

  const handleLogout = () => {
    localStorage.removeItem('token');
    localStorage.removeItem('user_id');
    navigate('/login');
  };

  return (
    <nav className="navbar">
      <div className="navbar-container">
        <Link to="/" className="navbar-logo">
          Go Commerce
        </Link>
        <div className="navbar-links">
          <Link to="/" className="navbar-link">
            首页
          </Link>
          <Link to="/products" className="navbar-link">
            商品
          </Link>
          <Link to="/merchants" className="navbar-link">
            商户管理
          </Link>
          {token ? (
            <>
              <Link to="/cart" className="navbar-link">
                购物车
              </Link>
              <Link to="/orders" className="navbar-link">
                订单
              </Link>
              <button className="navbar-logout" onClick={handleLogout}>
                退出登录
              </button>
            </>
          ) : (
            <>
              <Link to="/login" className="navbar-link">
                登录
              </Link>
              <Link to="/register" className="navbar-link">
                注册
              </Link>
            </>
          )}
        </div>
      </div>
    </nav>
  );
};

export default Navbar;